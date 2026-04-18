package service

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ks-prank/internal/consts"
	"ks-prank/internal/handler"
	"ks-prank/internal/protocol"
	mytypes "ks-prank/internal/types"
	"ks-prank/internal/worker"
	pb "ks-prank/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// PrankClient 所有平台整蛊客户端的统一接口
type PrankClient interface {
	Listen()
	Close()
}

// EventType 事件类型
type EventType string

const (
	EventGift    EventType = "gift"
	EventComment EventType = "comment"
	EventAction  EventType = "action"
	EventStatus  EventType = "status"
	EventLog     EventType = "log"
)

// EventPayload 推送给前端的事件
type EventPayload struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"timestamp"`
	Data      any       `json:"data"`
}

// GiftEventData 礼物事件数据
type GiftEventData struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	GiftName string `json:"gift_name"`
	Price    int    `json:"price"`
	Count    int    `json:"count"`
}

// CommentEventData 弹幕事件数据
type CommentEventData struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Content  string `json:"content"`
}

// EventCallback 事件回调函数
type EventCallback func(event EventPayload)

// KuaishouClient 快手直播间 WebSocket 客户端
type KuaishouClient struct {
	conn       *websocket.Conn
	dispatcher *worker.Dispatcher
	stopCh     chan struct{}
	stopOnce   sync.Once
	eventCb    EventCallback

	prank *mytypes.PrankConfigData

	giftTriggerMap map[string]*mytypes.GiftTrigger
	chatTriggerMap map[string]*mytypes.ChatTrigger

	// like 累计
	likeAccum atomic.Uint64
}

// NewKuaishouClient 创建客户端
func NewKuaishouClient(prank *mytypes.PrankConfigData, eventCb EventCallback) *KuaishouClient {
	kc := &KuaishouClient{
		stopCh:         make(chan struct{}),
		eventCb:        eventCb,
		prank:          prank,
		dispatcher:     worker.NewDispatcher(100),
		giftTriggerMap: make(map[string]*mytypes.GiftTrigger),
		chatTriggerMap: make(map[string]*mytypes.ChatTrigger),
	}
	if prank != nil {
		for i := range prank.GiftTriggers {
			t := &prank.GiftTriggers[i]
			if t.GiftName != "" {
				kc.giftTriggerMap[t.GiftName] = t
			}
		}
		for i := range prank.ChatTriggers {
			t := &prank.ChatTriggers[i]
			if t.Keyword != "" {
				kc.chatTriggerMap[t.Keyword] = t
			}
		}
	}
	return kc
}

// pickChoice 按权重随机挑一个 choice（权重和为 0 时退化到第一个）
func pickChoice(choices []mytypes.ActionChoice) *mytypes.ActionChoice {
	if len(choices) == 0 {
		return nil
	}
	total := 0
	for _, c := range choices {
		total += c.Weight
	}
	if total <= 0 {
		return &choices[0]
	}
	r := rand.Intn(total)
	cum := 0
	for i := range choices {
		cum += choices[i].Weight
		if r < cum {
			return &choices[i]
		}
	}
	return &choices[len(choices)-1]
}

func (kc *KuaishouClient) dispatchChoice(name string, hctx handler.HandlerCtx, choice *mytypes.ActionChoice) {
	if choice == nil {
		return
	}
	c := *choice
	kc.dispatcher.Dispatch(worker.Task{
		Name:        name,
		WorkerGroup: c.WorkerGroup,
		Run: func() {
			if err := handler.RunChoice(hctx, c); err != nil {
				log.Printf("执行 %s 失败: %v", c.Action, err)
			}
		},
	})
}

func (kc *KuaishouClient) emitEvent(eventType EventType, data any) {
	if kc.eventCb == nil {
		return
	}
	kc.eventCb(EventPayload{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	})
}

// Connect 连接快手直播间 WebSocket
func (kc *KuaishouClient) Connect(wssURL, token, liveStreamID string) error {
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Origin", "https://live.kuaishou.com")

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wssURL, headers)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	kc.conn = conn

	// 发送进房消息
	enterMsg, err := protocol.BuildEnterRoomMsg(token, liveStreamID)
	if err != nil {
		conn.Close()
		return fmt.Errorf("构建进房消息失败: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, enterMsg); err != nil {
		conn.Close()
		return fmt.Errorf("发送进房消息失败: %w", err)
	}

	log.Println("WebSocket 连接成功，已发送进房消息")
	kc.emitEvent(EventStatus, "connected")

	// 启动心跳
	go kc.heartbeatLoop()

	return nil
}

// Listen 阻塞监听消息，直到连接关闭或调用 Close
func (kc *KuaishouClient) Listen() {
	defer kc.emitEvent(EventStatus, "disconnected")

	for {
		select {
		case <-kc.stopCh:
			return
		default:
		}

		_, data, err := kc.conn.ReadMessage()
		if err != nil {
			if !strings.Contains(err.Error(), "close") {
				log.Printf("读取消息失败: %v", err)
			}
			return
		}
		kc.handleSocketMessage(data)
	}
}

// Close 断开连接
func (kc *KuaishouClient) Close() {
	kc.stopOnce.Do(func() {
		close(kc.stopCh)
		if kc.conn != nil {
			kc.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			kc.conn.Close()
		}
		kc.dispatcher.Stop()
		log.Println("快手客户端已关闭")
	})
}

func (kc *KuaishouClient) heartbeatLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-kc.stopCh:
			return
		case <-ticker.C:
			hbMsg, err := protocol.BuildHeartbeatMsg()
			if err != nil {
				log.Printf("构建心跳消息失败: %v", err)
				continue
			}
			if err := kc.conn.WriteMessage(websocket.BinaryMessage, hbMsg); err != nil {
				log.Printf("发送心跳失败: %v", err)
				return
			}
		}
	}
}

func (kc *KuaishouClient) handleSocketMessage(data []byte) {
	payloadType, payload, err := protocol.ParseSocketMessage(data)
	if err != nil {
		log.Printf("%v", err)
		return
	}

	switch payloadType {
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
		kc.handleFeedPush(feed)
	}
}

func (kc *KuaishouClient) handleFeedPush(feed *pb.SCWebFeedPush) {
	// 礼物
	for _, gift := range feed.GiftFeeds {
		user := gift.User
		userName := "未知用户"
		userAvatar := ""
		ksUid := ""
		if user != nil {
			userName = user.GetUserName()
			userAvatar = user.GetHeadUrl()
			ksUid = user.GetPrincipalId()
		}

		giftID := gift.GetGiftId()
		giftName := consts.GetGiftName(giftID)
		count := int(gift.GetBatchSize())
		if count <= 0 {
			count = 1
		}

		giftPrice := consts.GetGiftPrice(giftID)
		log.Printf("[礼物] %s 送出 %s (%d快币) x%d", userName, giftName, giftPrice, count)
		go handler.ReportKsGiftLog(ksUid, giftName, count, giftPrice, gift)

		kc.emitEvent(EventGift, GiftEventData{
			Username: userName,
			Avatar:   userAvatar,
			GiftName: giftName,
			Price:    giftPrice,
			Count:    count,
		})

		if trigger, ok := kc.giftTriggerMap[giftName]; ok {
			choice := pickChoice(trigger.Choices)
			kc.dispatchChoice(giftName, handler.HandlerCtx{
				Nickname:  userName,
				Avatar:    userAvatar,
				GiftCount: count,
			}, choice)
		}
	}

	// 弹幕
	for _, comment := range feed.CommentFeeds {
		user := comment.User
		userName := "未知用户"
		userAvatar := ""
		if user != nil {
			userName = user.GetUserName()
			userAvatar = user.GetHeadUrl()
		}

		content := comment.GetContent()
		log.Printf("[弹幕] %s: %s", userName, content)

		kc.emitEvent(EventComment, CommentEventData{
			Username: userName,
			Avatar:   userAvatar,
			Content:  content,
		})

		if trigger, ok := kc.chatTriggerMap[content]; ok {
			choice := pickChoice(trigger.Choices)
			kc.dispatchChoice("chat:"+content, handler.HandlerCtx{
				Nickname:  userName,
				Avatar:    userAvatar,
				GiftCount: 1,
			}, choice)
		}
	}

	// 点赞 —— 累计达到阈值即触发
	if kc.prank != nil && kc.prank.LikeTrigger != nil && kc.prank.LikeTrigger.Threshold > 0 {
		threshold := kc.prank.LikeTrigger.Threshold
		for _, like := range feed.LikeFeeds {
			_ = like
			nv := kc.likeAccum.Add(1)
			if nv >= threshold {
				kc.likeAccum.Store(0)
				choice := pickChoice(kc.prank.LikeTrigger.Choices)
				kc.dispatchChoice("like_threshold", handler.HandlerCtx{
					Nickname:  "观众们",
					Avatar:    "",
					GiftCount: 1,
				}, choice)
			}
		}
	}
}
