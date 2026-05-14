// Package stream 提供整蛊车 RTSP 流到本机 WebRTC 的桥接,供 ks-prank 桌面端预览。
//
// 视频路径走"帧级转发"(模拟 mediamtx 的做法):
// gortsplib 拉 RTSP H.264 RTP → rtph264.Decoder 重组 access unit →
// 拼 Annex-B → pion TrackLocalStaticSample.WriteSample → pion 重新打包。
// 这样浏览器拿到的是符合 WebRTC 习惯的 RTP 流,不会被 FU-A 分片边界
// 或半包卡住 jitter buffer。同时 IDR 帧若缺 SPS/PPS 会自动补上。
//
// 经过 A/B 验证: 这一步是消除卡顿的唯一必要条件。曾尝试过自定义
// MediaEngine 把 SDP 的 profile-level-id 调成源流真实值, 实测对稳定性
// 无帮助(浏览器 decoder 看的是流里的实际 SPS, 不是 SDP 声明),
// 所以保持 pion 默认 MediaEngine, 不再自定义。
//
// 音频路径(G.711 A-law) 包结构简单,继续 RTP 透传。
//
// 信令(SDP offer/answer) 由调用方通过函数参数传递,
// 对端是 WebView 里的 RTCPeerConnection, 两端全本机,无需 STUN/TURN。
package stream

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// Session 是一次 RTSP→WebRTC 桥接会话。
type Session struct {
	rtspClient *gortsplib.Client
	pc         *webrtc.PeerConnection
	closeOnce  sync.Once
	closed     chan struct{}
}

// Closed 返回一个 channel,会话结束(主动或被动)时关闭。
func (s *Session) Closed() <-chan struct{} {
	return s.closed
}

// Close 主动关闭会话,幂等。
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		if s.pc != nil {
			_ = s.pc.Close()
		}
		if s.rtspClient != nil {
			s.rtspClient.Close()
		}
	})
}

// Start 用整蛊车 IP 拉 RTSP, 配合浏览器侧的 offerSDP 建立本地 PeerConnection,
// 返回 Session 句柄和 answerSDP。调用方应在用户关闭预览时调用 Session.Close。
func Start(ctx context.Context, ip, offerSDP string) (*Session, string, error) {
	if net.ParseIP(ip) == nil {
		return nil, "", fmt.Errorf("无效的 IP: %q", ip)
	}
	rtspURL := fmt.Sprintf("rtsp://%s/live/0", ip)

	u, err := base.ParseURL(rtspURL)
	if err != nil {
		return nil, "", fmt.Errorf("解析 RTSP URL 失败: %w", err)
	}

	rc := &gortsplib.Client{}
	if err := rc.Start(u.Scheme, u.Host); err != nil {
		return nil, "", fmt.Errorf("连接 RTSP 失败: %w", err)
	}

	sess, _, err := rc.Describe(u)
	if err != nil {
		rc.Close()
		return nil, "", fmt.Errorf("RTSP DESCRIBE 失败: %w", err)
	}

	var h264Format *format.H264
	var h264Media *description.Media
	for _, m := range sess.Medias {
		for _, f := range m.Formats {
			if h, ok := f.(*format.H264); ok {
				h264Format = h
				h264Media = m
				break
			}
		}
		if h264Format != nil {
			break
		}
	}
	if h264Format == nil {
		rc.Close()
		return nil, "", fmt.Errorf("RTSP 流中没有 H264 视频")
	}

	var g711Format *format.G711
	var g711Media *description.Media
	for _, m := range sess.Medias {
		for _, f := range m.Formats {
			if g, ok := f.(*format.G711); ok {
				g711Format = g
				g711Media = m
				break
			}
		}
		if g711Format != nil {
			break
		}
	}

	if _, err := rc.Setup(sess.BaseURL, h264Media, 0, 0); err != nil {
		rc.Close()
		return nil, "", fmt.Errorf("RTSP SETUP 视频失败: %w", err)
	}
	if g711Media != nil {
		if _, err := rc.Setup(sess.BaseURL, g711Media, 0, 0); err != nil {
			log.Printf("[stream] RTSP SETUP 音频失败,继续只播视频: %v", err)
			g711Media = nil
			g711Format = nil
		}
	}

	// 诊断日志: 记下源流真实的 H.264 profile, 调试时方便对照浏览器协商的 SDP。
	// 注意 SDP 里 pion 默认仍宣称 42e01f, 实测不影响解码,见包文档说明。
	log.Printf("[stream] H.264 source profile-level-id=%s (SPS=%d bytes)",
		extractH264ProfileLevelID(h264Format.SPS), len(h264Format.SPS))

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		rc.Close()
		return nil, "", fmt.Errorf("创建 PeerConnection 失败: %w", err)
	}

	s := &Session{
		rtspClient: rc,
		pc:         pc,
		closed:     make(chan struct{}),
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[stream] PC 状态: %s", state)
		switch state {
		case webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateClosed:
			s.Close()
		}
	})

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH264,
			ClockRate: 90000,
		},
		"video", "ks-prank-car",
	)
	if err != nil {
		s.Close()
		return nil, "", fmt.Errorf("创建视频 track 失败: %w", err)
	}
	videoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		s.Close()
		return nil, "", fmt.Errorf("绑定视频 track 失败: %w", err)
	}
	// drain RTCP, 否则会堆积
	go drainRTCP(videoSender)

	h264Dec, err := h264Format.CreateDecoder()
	if err != nil {
		s.Close()
		return nil, "", fmt.Errorf("创建 H.264 解封装器失败: %w", err)
	}

	var audioTrack *webrtc.TrackLocalStaticRTP
	if g711Format != nil {
		audioTrack, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypePCMA,
				ClockRate: 8000,
				Channels:  1,
			},
			"audio", "ks-prank-car",
		)
		if err != nil {
			log.Printf("[stream] 创建音频 track 失败,放弃音频: %v", err)
			audioTrack = nil
		} else {
			audioSender, aerr := pc.AddTrack(audioTrack)
			if aerr != nil {
				log.Printf("[stream] 绑定音频 track 失败,放弃音频: %v", aerr)
				audioTrack = nil
			} else {
				go drainRTCP(audioSender)
			}
		}
	}

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		s.Close()
		return nil, "", fmt.Errorf("setRemoteDescription 失败: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		s.Close()
		return nil, "", fmt.Errorf("createAnswer 失败: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		s.Close()
		return nil, "", fmt.Errorf("setLocalDescription 失败: %w", err)
	}

	select {
	case <-gatherComplete:
	case <-ctx.Done():
		s.Close()
		return nil, "", ctx.Err()
	}

	// SPS/PPS 缓存: SDP 已经带了一份;如果流中又出现,会被更新。
	// 浏览器在 IDR 之前必须见到 SPS/PPS, 否则解码器初始化不了。
	var (
		spsBytes      = append([]byte(nil), h264Format.SPS...)
		ppsBytes      = append([]byte(nil), h264Format.PPS...)
		lastFrameTime time.Time
	)

	rc.OnPacketRTP(h264Media, h264Format, func(pkt *rtp.Packet) {
		au, derr := h264Dec.Decode(pkt)
		if derr != nil {
			if !errors.Is(derr, rtph264.ErrMorePacketsNeeded) &&
				!errors.Is(derr, rtph264.ErrNonStartingPacketAndNoPrevious) {
				log.Printf("[stream] H.264 解封装失败: %v", derr)
			}
			return
		}

		// 更新 SPS/PPS 缓存, 同时判断是不是 IDR 帧 / 是否含参数集
		var hasIDR, hasSPS, hasPPS bool
		for _, n := range au {
			if len(n) == 0 {
				continue
			}
			switch n[0] & 0x1F {
			case 5:
				hasIDR = true
			case 7:
				hasSPS = true
				spsBytes = append(spsBytes[:0], n...)
			case 8:
				hasPPS = true
				ppsBytes = append(ppsBytes[:0], n...)
			}
		}

		// 若是 IDR 但流里没有伴随 SPS/PPS, 用缓存补在最前面
		var output [][]byte
		if hasIDR && !hasSPS && !hasPPS && len(spsBytes) > 0 && len(ppsBytes) > 0 {
			output = append(output, spsBytes, ppsBytes)
		}
		output = append(output, au...)

		// 拼 Annex-B (NALU 之间用 00 00 00 01 分隔)
		var buf bytes.Buffer
		for _, n := range output {
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(n)
		}

		// Duration 用 wall clock 实时近似帧间隔, 用于 pion 推 RTP 时间戳
		now := time.Now()
		var duration time.Duration
		if !lastFrameTime.IsZero() {
			duration = now.Sub(lastFrameTime)
		} else {
			duration = 33 * time.Millisecond
		}
		lastFrameTime = now

		if err := videoTrack.WriteSample(media.Sample{
			Data:     buf.Bytes(),
			Duration: duration,
		}); err != nil && err != io.ErrClosedPipe {
			log.Printf("[stream] 视频 sample 写入失败: %v", err)
		}
	})
	if audioTrack != nil && g711Format != nil {
		isStereo := g711Format.ChannelCount >= 2
		rc.OnPacketRTP(g711Media, g711Format, func(pkt *rtp.Packet) {
			if isStereo && len(pkt.Payload) >= 2 {
				// 双声道交错(LRLR...)取左声道转单声道
				mono := make([]byte, len(pkt.Payload)/2)
				for i := range mono {
					mono[i] = pkt.Payload[i*2]
				}
				pkt.Payload = mono
			}
			if err := audioTrack.WriteRTP(pkt); err != nil && err != io.ErrClosedPipe {
				log.Printf("[stream] 音频 RTP 写入失败: %v", err)
			}
		})
	}

	if _, err := rc.Play(nil); err != nil {
		s.Close()
		return nil, "", fmt.Errorf("RTSP PLAY 失败: %w", err)
	}

	return s, pc.LocalDescription().SDP, nil
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

// extractH264ProfileLevelID 从 SPS 抽 profile-level-id (6 hex chars), 用于诊断日志:
//
//	SPS layout: [NAL header byte][profile_idc][constraint_flags][level_idc][...]
//
// 拿到的 3 字节按 RFC 6184 拼成 profile-level-id。SPS 异常时回退到
// constrained baseline 3.1 (42e01f), 与 pion 默认值一致。
func extractH264ProfileLevelID(sps []byte) string {
	if len(sps) < 4 {
		return "42e01f"
	}
	return hex.EncodeToString(sps[1:4])
}
