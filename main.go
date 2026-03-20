package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	pb "ks-prank/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// 快手礼物ID -> 名称映射（常见礼物）
var giftNames = map[uint32]string{
	1: "荧光棒", 2: "棒棒糖", 3: "冰淇淋", 4: "香蕉",
	5: "大鸡腿", 6: "啤酒", 7: "彩虹", 8: "冰淇淋",
	9: "西瓜", 10: "仙女棒", 11: "红玫瑰", 12: "女王皇冠",
	13: "兰博基尼", 14: "大飞机", 15: "游艇", 16: "摩天轮",
	17: "城堡", 18: "蓝色妖姬", 19: "520", 20: "小红心",
	21: "幸运星", 22: "礼花", 23: "告白气球", 24: "相守一生",
	25: "比心", 26: "手枪", 27: "一杯敬远方", 28: "遇见你很开心",
	30: "求关注", 31: "加油鸭", 32: "暴击三连", 33: "红包",
	35: "给力", 36: "真好听", 37: "老铁双击666",
	40: "飞吻", 41: "跑车", 42: "蹦迪", 43: "穿越机",
	44: "烟花", 45: "热气球", 46: "星河入梦", 47: "一生有你",
	101: "幸运喜袋", 113: "火箭", 114: "飞机", 281: "私人飞机",
}

func getGiftName(id uint32) string {
	if name, ok := giftNames[id]; ok {
		return name
	}
	return fmt.Sprintf("未知礼物(%d)", id)
}

func generatePageID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s_%d", string(b), time.Now().UnixMilli())
}

func buildEnterRoomMsg(token, liveStreamID string) ([]byte, error) {
	enterRoom := &pb.CSWebEnterRoom{
		Token:        proto.String(token),
		LiveStreamId: proto.String(liveStreamID),
		PageId:       proto.String(generatePageID()),
	}
	enterPayload, err := proto.Marshal(enterRoom)
	if err != nil {
		return nil, err
	}

	msg := &pb.SocketMessage{
		PayloadType:     pb.PayloadType_CS_ENTER_ROOM.Enum(),
		CompressionType: pb.SocketMessage_NONE.Enum(),
		Payload:         enterPayload,
	}
	return proto.Marshal(msg)
}

func buildHeartbeatMsg() ([]byte, error) {
	hb := &pb.CSWebHeartbeat{
		Timestamp: proto.Uint64(uint64(time.Now().UnixMilli())),
	}
	hbPayload, err := proto.Marshal(hb)
	if err != nil {
		return nil, err
	}

	msg := &pb.SocketMessage{
		PayloadType:     pb.PayloadType_CS_HEARTBEAT.Enum(),
		CompressionType: pb.SocketMessage_NONE.Enum(),
		Payload:         hbPayload,
	}
	return proto.Marshal(msg)
}

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func handleSocketMessage(data []byte) {
	msg := &pb.SocketMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		log.Printf("解析 SocketMessage 失败: %v", err)
		return
	}

	payload := msg.Payload
	if msg.CompressionType != nil && *msg.CompressionType == pb.SocketMessage_GZIP {
		var err error
		payload, err = decompressGzip(payload)
		if err != nil {
			log.Printf("GZIP 解压失败: %v", err)
			return
		}
	}

	switch msg.GetPayloadType() {
	case pb.PayloadType_SC_ENTER_ROOM_ACK:
		ack := &pb.SCWebEnterRoomAck{}
		if err := proto.Unmarshal(payload, ack); err == nil {
			log.Printf("[进房确认] code=%d msg=%s", ack.GetCode(), ack.GetMsg())
		}

	case pb.PayloadType_SC_HEARTBEAT_ACK:
		// 心跳回复，静默处理

	case pb.PayloadType_SC_FEED_PUSH:
		feed := &pb.SCWebFeedPush{}
		if err := proto.Unmarshal(payload, feed); err != nil {
			log.Printf("解析 SCWebFeedPush 失败: %v", err)
			return
		}
		handleFeedPush(feed)

	default:
		log.Printf("[未知消息] payloadType=%d, payload长度=%d", msg.GetPayloadType(), len(payload))
	}
}

func handleFeedPush(feed *pb.SCWebFeedPush) {
	// 处理礼物
	for _, gift := range feed.GiftFeeds {
		user := gift.User
		userName := "未知用户"
		if user != nil {
			userName = user.GetUserName()
		}
		giftName := getGiftName(gift.GetGiftId())
		log.Printf("[礼物] %s 送出 %s (giftId=%d, batchSize=%d, comboCount=%d, starLevel=%d)",
			userName, giftName,
			gift.GetGiftId(), gift.GetBatchSize(), gift.GetComboCount(), gift.GetStarLevel())
	}

	// 处理弹幕
	for _, comment := range feed.CommentFeeds {
		user := comment.User
		userName := "未知用户"
		if user != nil {
			userName = user.GetUserName()
		}
		log.Printf("[弹幕] %s: %s", userName, comment.GetContent())
	}

	// 处理点赞（量大，只在有礼物/弹幕时顺带打印）
	if len(feed.LikeFeeds) > 0 && (len(feed.GiftFeeds) > 0 || len(feed.CommentFeeds) > 0) {
		log.Printf("[点赞] 本次推送包含 %d 条点赞", len(feed.LikeFeeds))
	}
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Demo 模式：直接硬编码连接信息，验证协议是否能跑通
	wssURL := "wss://livejs-ws-group5.gifshow.com/websocket"
	token := "Hxfj8WpSi0ay9wmnSTtJBWBO5DOHLZ0twR4dKYbxd+GFFnM3r1fFubfAsRk1fJM2sqRfrQuRM0C86rzGUfWW7G2xWifTHM40qPSwlzPZMJMGtZYdFHSIbYuKptIIBFulAi/hdlQH0siF2/VBQ0VznnvCHIozIsBly2ZBMB93Y6RgViHe/XdyhYLrO8VG9JMeVL4bE5p2Y5BqY62whzKU3mlmj7tGA9xWVX7yCC7u1eY="
	liveStreamID := "nWRydQ_je14"

	// 允许通过命令行覆盖
	if len(os.Args) >= 4 {
		wssURL = os.Args[1]
		token = os.Args[2]
		liveStreamID = os.Args[3]
	}

	log.Printf("liveStreamId: %s", liveStreamID)
	log.Printf("WSS URL: %s", wssURL)

	// 连接 WebSocket
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Origin", "https://live.kuaishou.com")

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wssURL, headers)
	if err != nil {
		log.Fatalf("WebSocket 连接失败: %v", err)
	}
	defer conn.Close()
	log.Println("WebSocket 连接成功")

	// 发送进房消息
	enterMsg, err := buildEnterRoomMsg(token, liveStreamID)
	if err != nil {
		log.Fatalf("构建进房消息失败: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, enterMsg); err != nil {
		log.Fatalf("发送进房消息失败: %v", err)
	}
	log.Println("已发送进房消息")

	// 启动心跳
	stopHB := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHB:
				return
			case <-ticker.C:
				hbMsg, err := buildHeartbeatMsg()
				if err != nil {
					log.Printf("构建心跳消息失败: %v", err)
					continue
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, hbMsg); err != nil {
					log.Printf("发送心跳失败: %v", err)
					return
				}
			}
		}
	}()

	// 监听退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	// 接收消息
	msgCh := make(chan []byte, 100)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "close") {
					log.Printf("读取消息失败: %v", err)
				}
				close(msgCh)
				return
			}
			msgCh <- data
		}
	}()

	log.Println("开始监听快手直播间消息...")
	for {
		select {
		case <-sigCh:
			log.Println("收到退出信号，正在关闭...")
			close(stopHB)
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case data, ok := <-msgCh:
			if !ok {
				log.Println("连接已断开")
				close(stopHB)
				return
			}
			handleSocketMessage(data)
		}
	}
}
