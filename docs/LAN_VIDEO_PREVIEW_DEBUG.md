# 整蛊车 LAN 视频预览：从卡顿到流畅的实战调试

## 一、起点

ks-prank 桌面端需要在主播侧本地预览整蛊车的画面。约束有两条：

- **必须 LAN 直连** — 不能像生产链路那样走公网中转，避免网络抖动影响主播体验
- **彻底去除外部二进制依赖** — 最初的方案是 spawn `ffplay` 子进程，要替换成应用进程内自闭环

可参考的资源：

| 资源 | 价值 |
|------|------|
| 车端 `luck-pets-car/internal/goroutines/publishWebrtcStream.go` | 已经用 `gortsplib + pion/webrtc` 把车端 RTSP 推成 WebRTC，结构可以照抄 |
| 生产链路 (car → 公网 mediamtx WHIP → 浏览器 WHEP) | 跑了一年多稳定，**这是关键的"对照组"** |

---

## 二、第一版：照抄车端，只换信令

把车端那 200 多行代码搬过来，只改三处：

1. RTSP 源从 `127.0.0.1` 改成 LAN IP
2. 信令去掉 WHIP HTTP POST，改用 Wails 绑定函数本机交换 SDP
3. pion 配置去掉 ICE servers（全本机不需要 STUN/TURN）

代码：

```go
videoTrack, _ := webrtc.NewTrackLocalStaticRTP(
    webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
    "video", "ks-prank-car",
)
pc.AddTrack(videoTrack)

rc.OnPacketRTP(h264Media, h264Format, func(pkt *rtp.Packet) {
    videoTrack.WriteRTP(pkt) // RTSP RTP 包原封不动透传给 WebRTC
})
```

**画面通了** —— 这一步意义重大，证明 H.264 解码、WebRTC 协商、信令交换都没问题。

### 思路要点

> **先把核心路径打通，不要一上来就追求完美**。哪怕最 raw 的方案能出一帧画面，也比纸上谈兵推演十种方案有价值得多。

---

## 三、第一个症状：周期性卡顿

跑起来发现画面每过几秒就卡 2-3 秒，恢复后过几秒又卡，循环往复。

这是 WebRTC 里最经典的症状：**decoder 解码失败，丢弃所有后续帧直到下一个 IDR (keyframe)**。

接下来三轮猜测假设，**没一个解决了问题**。

### 假设 1：UDP 丢包

WiFi 上 UDP 必然有损，丢一个 P 帧就要等下一个 IDR。仿照原 ffplay 命令的 `-rtsp_transport tcp`，把 gortsplib 切到 TCP。

```go
transport := gortsplib.TransportTCP
rc := &gortsplib.Client{Transport: &transport}
```

**没解决卡顿，而且引入了"完全黑屏"问题。**这一步事后回退了。

### 假设 2：H.264 profile 不匹配

pion 默认 H.264 codec 写死 `profile-level-id=42e01f`（Constrained Baseline 3.1）。如果车端推的是更高 profile，浏览器一开始凑合解，遇到 profile 特性帧就崩。

从 SPS 里抠真实 profile：

```go
func extractH264ProfileLevelID(sps []byte) string {
    if len(sps) < 4 { return "42e01f" }
    return hex.EncodeToString(sps[1:4]) // profile_idc + constraint + level_idc
}
```

打出来：`profile-level-id=641028` —— **High Profile, Level 4.0**，跟默认差两档，看着像凶手。配上自定义 MediaEngine、注册 NACK/PLI feedback。

**仍然卡。**

### 假设 3：差点换工具

我提议换成本机 spawn 一个 mediamtx 或 go2rtc。被用户阻止：

> **不换，我走公网这一套技术路线都跑通了，我不信局域网跑不通，你再想想办法**

事后看，这是整个调试里最关键的一句话。

---

## 四、真正的修复：模仿 mediamtx 做帧级转发

被怼回来后重新审视"为什么生产链路稳定"：

```
车端 gortsplib (RTSP → pion) ──WHIP──> 公网 mediamtx ──WHEP──> 浏览器
                                          ▲
                                          │ 真正为浏览器做兼容工作的是它
```

**盲点**：我之前把车端那段 pion 代码当成"已经搞定了把 RTP 喂给 WebRTC 这件事"的 reference。**它没搞定**。它的对端是 mediamtx，mediamtx 会做：

1. **拆包重组** — 把 FU-A 分片的 H.264 RTP 重组成完整 access unit（一帧）
2. **缓存 SPS/PPS** — 必要时给 IDR 帧前面补上参数集
3. **重新打包** — 按 WebRTC 习惯把帧打成 RTP 发给浏览器

我之前是 raw RTP 透传，浏览器拿到的是 RTSP 风格的 RTP。WebRTC jitter buffer 偶尔判定帧不完整就丢弃，丢完等下一个 IDR —— 完美对应"每几秒卡 2-3 秒"。

复刻 mediamtx 的关键步骤：

```go
// gortsplib 的 H264 RTP 解封装器, 累积 FU-A 分片直到拿到完整 access unit
au, derr := h264Dec.Decode(pkt)
if errors.Is(derr, rtph264.ErrMorePacketsNeeded) { return } // 等下一包

// 维护 SPS/PPS 缓存, IDR 缺参数集就补上
if hasIDR && !hasSPS && !hasPPS { au = prepend(spsBytes, ppsBytes, au) }

// 拼 Annex-B → 让 pion 自己重新打包成 WebRTC RTP
videoTrack.WriteSample(media.Sample{
    Data:     annexB(au),
    Duration: time.Since(lastFrameTime),
})
```

`TrackLocalStaticSample` 内部走 pion 实现的 H.264 packetizer：正确的 FU-A 分片、marker 位、单调时间戳。浏览器看到的就是教科书般规整的 WebRTC RTP。

**完全不卡了。**

### 思路要点

> **"能跑" ≠ "通用"**。当一段代码在某个上下文里稳定运行，别假设它在新上下文里也行。要先搞清楚那个上下文里有什么东西在帮你兜底。本案的兜底是 mediamtx，在车端代码里你看不到它，但它做的事情决定了浏览器看到的流的形态。

---

## 五、转折：差点错下结论

我兴奋地下了结论："**帧级转发解决了问题**"，准备写笔记。

但用户提了一个不舒服的问题：

> "udp 改 tcp 之后就完全黑屏，再到成功，中间经历了两步，1 是 profile 匹配 2 是模仿 mediamtx 帧级转发，那应该没法判断谁是解决问题的核心因素吧？"

复盘时间线：

| 阶段 | 传输 | profile 修正 | 帧级转发 | 结果 |
|------|------|--------------|----------|------|
| 0 | UDP | 默认 `42e01f` | ❌ | 周期性卡顿 |
| 1 | **TCP** | 默认 | ❌ | 还卡 + 黑屏 |
| 2 | TCP | **真实 `641028`** | ❌ | 还卡 |
| 3 | UDP(回退) | 真实 | **✅** | 完全流畅 |

阶段 3 同时回退 TCP **并且**加了帧级转发。**没有任何一组实验单独验证过 profile 或帧级是不是必要的**。我把功劳归给"帧级转发 + profile 修正"组合，是过度归因。

用户的另一句话补刀：

> "我很想知道谁是根本解决方案，但又怕把代码改坏了回不来，你想想办法"

这恰恰是 git 最擅长的场景。

---

## 六、A/B 隔离：用 git 兜底跑两次实验

流程很简单：

```
Step 1  把当前能跑的版本 commit 进 git    ← 不可摧毁的回退点
Step 2  实验 A: 关帧级、留 profile         → 测、报告
        git checkout HEAD -- car_stream.go ← 还原
Step 3  实验 B: 关 profile、留帧级         → 测、报告
        git checkout HEAD -- car_stream.go ← 还原
Step 4  按结论简化代码, 二次 commit
```

只要 Step 1 commit 做了，后面所有实验都是无风险的 —— 哪怕代码改到无法编译，一句 `git checkout HEAD --` 一秒还原。

### 实验 A：关帧级、留 profile

回到 `TrackLocalStaticRTP` raw RTP 透传，但 SDP 仍声明真实 profile-level-id。

**结果：完全黑屏，连卡顿的画面都看不到。**

### 实验 B：关 profile、留帧级

去掉自定义 MediaEngine，SDP 用 pion 默认（声明 `42e01f`），但保留 `TrackLocalStaticSample` 帧级转发。

**结果：完全流畅。**

### 最终结论矩阵

| 实验 | 帧级 | profile | 结果 |
|------|------|---------|------|
| 原始 | ❌ | 默认 | 卡顿 |
| A | ❌ | 真实 | **黑屏（更糟）** |
| B | ✅ | 默认 | **流畅** |
| 工作版 | ✅ | 真实 | 流畅 |

**真正的 hero 是帧级 depacketize/repacketize**。profile 修正不仅不必要，**在没有帧级转发时还会让画面更糟**（推测：SDP 声明高 profile 让浏览器 decoder 切到更严格模式，对 raw RTSP RTP 输入更扛不住）。

### 为什么 profile 修正没用？

浏览器 H.264 decoder 看的是流里实际的 SPS，不是 SDP 声明。SDP 上的 `profile-level-id` 只是协商时双方互相承诺"我能处理这个 profile"，但 decoder 实际工作时按收到的 SPS 重新配置。所以 SDP 声明跟流真实 profile 不一致，decoder 也能干活。

把 ~50 行自定义 MediaEngine 代码删掉，commit 一次最终版本。

### 思路要点

> **多变量同时改后必须 A/B 隔离**。任何"看起来这个 fix 起作用了"的判断都可能是错的，尤其是中间步骤还做了别的修改时。git 让 A/B 实验几乎零成本（commit + checkout），没理由不做。

> **用户的"再想想办法"是最有价值的反馈**。如果他没在"假设 3"打断我，我们会换成 mediamtx，绕过这次学习、引入外部依赖、违背最初目标。

---

## 七、总结：通用方法论

### 这次踩的坑

1. **用错了 reference**：车端 pion 代码不是"RTP 喂给 WebRTC"的标准实现，它依赖 mediamtx 做下游兜底
2. **没单变量验证**：profile + 帧级一起改，看到画面好就停手
3. **被症状误导**：看到周期性卡顿就联想到丢包，看到 profile 不匹配就觉得是凶手 —— 都是合理的假设但都不是真相

### 沉淀下来的判断准则

| 场景 | 怎么办 |
|------|--------|
| 用户拿出"另一条同栈链路是通的" | 这是强信号：去复刻那条链路的关键步骤，不要切换到更厚的工具 |
| 一次修改改了多个变量后症状消失 | 必须 A/B 隔离，否则可能把功劳错给中间步骤 |
| 担心实验把代码改坏 | 先 commit 一个回退点，git checkout 兜底，实验就是零成本的 |
| RTSP → 浏览器需要桥接 | 务必做 depacketize→repacketize，raw RTP 透传不稳定。这不是性能优化，是稳定性的必要条件 |

### 最终落地的代码

只动了一个文件 `internal/stream/car_stream.go`：

- gortsplib 拉 RTSP H.264 + G.711
- `rtph264.Decoder` 累积 FU-A 拼出完整 access unit
- 维护 SPS/PPS 缓存，IDR 缺参数集就补
- pion `TrackLocalStaticSample` 让 pion 重新打包成 WebRTC RTP
- 音频走 raw RTP 透传（G.711 包结构简单，没这个问题）
- PeerConnection 全部走 pion 默认

最终代码比工作版还少了 49 行。

---

## 八、相关 commit

| Commit | 内容 |
|--------|------|
| `9976173` | feat: 整蛊车 LAN RTSP→WebRTC 桥接（带冗余 profile 修正） |
| `12067dc` | refactor: A/B 实测确认 profile 修正无用,删掉 64 行,加 15 行说明 |

---

## 九、扫盲：profile 和帧级转发的原理

### Q1. profile 是啥东西？都已知是 H.264 了还有啥不一样？

**H.264 是一个标准，但标准里的"可选功能"是分组的，每一组叫一个 profile**。可以理解成"H.264 这套规范有一百多个特性，把它们打成几个包，方便不同档次的硬件挑着支持"。

常见 profile：

| profile | profile_idc (hex) | 典型用途 | 关键特性 |
|---------|-------------------|----------|----------|
| Baseline | `0x42` (66) | 视频通话、低端硬件 | 只有 I/P 帧，无 B 帧，无 CABAC |
| Constrained Baseline | `0x42` + 约束 flag | 最广泛兼容 | Baseline 的子集，限制更严 |
| Main | `0x4d` (77) | 标清广播、早期 DVD | 加 B 帧、CABAC 熵编码 |
| **High** | `0x64` (100) | 1080p 流媒体、Blu-ray、本案车端 | 加 8×8 变换、自定义量化矩阵、单色支持 |

每个 profile 还要配一个 **level**，表示分辨率 / 码率 / 帧率的上限：

| level (hex) | 大致能力 |
|------|----------|
| 3.0 (`0x1e`) | 720p30 |
| 3.1 (`0x1f`) | 720p30 高码率 |
| **4.0 (`0x28`)** | 1080p30 (本案车端) |
| 4.1 (`0x29`) | 1080p30 高码率 |

SDP 里的 `profile-level-id` 就是 **3 字节十六进制串**，分别是 `profile_idc | constraint_flags | level_idc`：

- `42e01f` = `0x42`(Baseline) + `0xe0`(全部约束 → Constrained Baseline) + `0x1f`(Level 3.1)，最弱、最普及，pion 默认就给这个
- `4d401f` = Main @ 3.1
- **`641028`** = High @ 4.0，本案车端实际推的就是这个

**为什么不同 profile 不能互相替代**：高 profile 用的特性(比如 B 帧、CABAC、8×8 变换)，低 profile 的 decoder 根本没实现。所以理论上：

- High 编码 → Baseline decoder 解：**会失败**，遇到不认识的语法元素就崩
- Baseline 编码 → High decoder 解：**没问题**，High 是超集

**那为什么我们写错 profile 浏览器还能解？** 因为：

1. **浏览器 H.264 decoder 在工程实现上通常是"全能"的** —— 即使 SDP 协商时只承诺 Baseline，底层调用的 ffmpeg/VideoToolbox/MediaFoundation 都能处理到 High，浏览器只是用 SDP profile 判断"要不要接受这个流"，实际解码时按 SPS 走。
2. **SPS 是流里最权威的元信息**。每个 H.264 流里都内嵌一个 SPS，记着真正的 profile/level/分辨率。decoder 拿到关键帧时会按 SPS 重新配置自己。SDP 上写啥都没那么重要。

所以本案"自定义 profile-level-id 写真实值"看着合理，实际多此一举。

---

### Q2. 为什么帧级转发能消除卡顿？

要看懂这个得先理解 **一帧 H.264 视频是怎么变成 RTP 包的**。

#### RTP 不是按帧切的，是按 1400 字节切的

RTP 跑在 UDP 之上（或者 TCP-over-RTSP），单个包受网络 MTU 限制，**实际最多带 1400 字节左右** payload。但一帧 H.264 视频，尤其是关键帧 (IDR)，常常 50-200KB。

所以一帧要拆成几十到上百个 RTP 包。RFC 6184 (H.264-over-RTP) 定义了三种打法：

| 打法 | 含义 |
|------|------|
| Single NAL | 小帧/小 NALU 直接整个塞一个 RTP 包 |
| **FU-A**(Fragmentation Unit) | 大 NALU 切片，每片塞一个 RTP 包，**包头里有 start/end bit 标记**这是第几片 |
| STAP-A (Aggregation) | 多个小 NALU 拼一个 RTP 包，节省包头开销 |

实际场景里大帧靠 FU-A，小帧靠 Single NAL 或 STAP-A。

#### 浏览器 jitter buffer 在拼什么？

浏览器拿到一堆 RTP 包,要做几件事:

1. **按 sequence number 重排序**(UDP 可能乱序)
2. **按 RTP timestamp 分组**(同一 timestamp 的包属于同一帧)
3. **检查 FU-A 的 start/end bit 是否齐全**,缺一片就拼不成一个 NALU
4. **看 marker bit 判断一帧是否结束**(RTP header 有 1 bit 标记)
5. 拼成完整帧才喂给 decoder

如果第 3 步或第 4 步出问题(缺一片 FU-A、marker 没设对),这一帧**直接整个丢弃**,decoder 就丢了一个 P 帧的预测参考,**之后的 P 帧也没法解了,要等下一个 IDR**。

IDR 通常每 1-5 秒一个 —— **这就是"卡 2-3 秒后恢复"症状的原理**。

#### raw RTP 透传错在哪?

我们最开始的做法:gortsplib 收到 RTP → pion `TrackLocalStaticRTP.WriteRTP(pkt)` → 浏览器。**这是在转发别人(RTSP 摄像头)的包,只改了 SRTP 加密层**。

但 RTSP 摄像头的 RTP 打法和 WebRTC 习惯**不完全合拍**:

- 摄像头打 FU-A 的最大分片可能跟 WebRTC 期望的不一致
- marker bit 的设置时机可能跟 WebRTC 假设有微妙差异
- 时间戳节奏由摄像头决定,跟 pion 内部 SRTP 加密的处理时序不一定对齐
- **session 重启时 sequence number 会跳变**

这些差异在 mediamtx 那种宽容服务器里是没问题的(它有自己更复杂的 jitter 容忍逻辑),**但浏览器 jitter buffer 比较严格**,偶尔判定一帧不完整就丢,丢了就要等下个 IDR。

#### 帧级转发解决了什么?

```
RTSP RTP 包 → rtph264.Decoder 累积 → 完整 access unit → pion 重新打 RTP → 浏览器
                    ▲                       ▲                  ▲
                按 FU-A 重组             一帧 H.264         pion 用自己的 H.264
                                          完整字节             packetizer 重新打包
```

`TrackLocalStaticSample.WriteSample(media.Sample{...})` 这个 API 内部走 pion 实现的 packetizer:

- **重新分配 sequence number**(连续、单调)
- **按 pion 默认习惯做 FU-A 切片**(跟 pion-side WebRTC 假设一致)
- **正确设置 marker bit**(只有 access unit 的最后一个包置 1)
- **按 Sample.Duration 推时间戳**(单调递增)

浏览器 jitter buffer 看到的是"教科书般规整的 WebRTC RTP",拼帧没难度,解码就稳了。

#### 那 mediamtx 在生产链路里做的就是这件事

```
车端 → 公网 mediamtx (做了帧级 depacketize + repacketize) → 浏览器
```

所以"车端 pion 代码能稳定推到 mediamtx"不等于"车端 pion 代码能稳定推给浏览器" —— **中间那个 mediamtx 是关键**,它替浏览器把流"洗"干净了。

我们 LAN 直连版要复刻的就是 mediamtx 的"洗流"动作。

### 一句话总结

- **profile** = H.264 标准内部的功能子集分组,SDP 上写的是协商承诺,实际 decoder 看流里的 SPS。所以 SDP 写错通常不致命。
- **帧级转发** = 把"别人发来的 RTP 包"重新拆成完整帧再用自家 packetizer 打包,目的是让下游(浏览器)拿到符合它习惯的 RTP 流,避免它的 jitter buffer 误判帧不完整。
