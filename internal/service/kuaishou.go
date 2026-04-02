package service

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"ks-prank/config"
	"ks-prank/internal/consts"
	"ks-prank/internal/handler"
	"ks-prank/internal/protocol"
	"ks-prank/internal/worker"
	pb "ks-prank/proto"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

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
	dispatcher *worker.GiftDispatcher
	stopCh     chan struct{}
	stopOnce   sync.Once
	eventCb    EventCallback

	// 弹幕随机
	chatTrigger     string
	chatEntries     []chatActionEntry
	chatTotalWeight int
}

type chatActionEntry struct {
	virtualGiftName string
	weight          int
}

// NewKuaishouClient 创建客户端
func NewKuaishouClient(cfg *config.Config, eventCb EventCallback) (*KuaishouClient, error) {
	kc := &KuaishouClient{
		stopCh:  make(chan struct{}),
		eventCb: eventCb,
	}

	dispatcher, err := kc.buildDispatcher(cfg)
	if err != nil {
		return nil, err
	}
	kc.dispatcher = dispatcher

	return kc, nil
}

func (kc *KuaishouClient) buildDispatcher(cfg *config.Config) (*worker.GiftDispatcher, error) {
	actions := make(map[string]worker.GiftAction)
	groupMap := make(map[int][]string)

	for _, ga := range cfg.GiftActions {
		factory, ok := handler.ActionRegistry[ga.Action]
		if !ok {
			return nil, fmt.Errorf("未知 action: %s (礼物: %s)", ga.Action, ga.GiftName)
		}
		actions[ga.GiftName] = factory(ga)
		groupMap[ga.WorkerGroup] = append(groupMap[ga.WorkerGroup], ga.GiftName)
	}

	if cfg.ChatAction != nil && cfg.ChatAction.Enabled {
		kc.chatTrigger = cfg.ChatAction.Trigger
		for i, entry := range cfg.ChatAction.Actions {
			virtualName := fmt.Sprintf("_chat_%d", i)
			fakeCfg := config.GiftActionConfig{
				GiftName: virtualName,
				Action:   entry.Action,
				Params:   entry.Params,
			}
			factory, ok := handler.ActionRegistry[entry.Action]
			if !ok {
				return nil, fmt.Errorf("弹幕随机: 未知 action: %s", entry.Action)
			}
			actions[virtualName] = factory(fakeCfg)
			groupMap[entry.WorkerGroup] = append(groupMap[entry.WorkerGroup], virtualName)

			kc.chatEntries = append(kc.chatEntries, chatActionEntry{
				virtualGiftName: virtualName,
				weight:          entry.Weight,
			})
			kc.chatTotalWeight += entry.Weight
		}
	}

	maxGroup := 0
	for g := range groupMap {
		if g > maxGroup {
			maxGroup = g
		}
	}
	giftGroups := make([][]string, maxGroup+1)
	for g, names := range groupMap {
		giftGroups[g] = names
	}

	return worker.NewGiftDispatcher(actions, giftGroups, 100), nil
}

func (kc *KuaishouClient) pickRandomChatAction() string {
	r := rand.Intn(kc.chatTotalWeight)
	cumulative := 0
	for _, e := range kc.chatEntries {
		cumulative += e.weight
		if r < cumulative {
			return e.virtualGiftName
		}
	}
	return kc.chatEntries[len(kc.chatEntries)-1].virtualGiftName
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

	// 启动 dispatcher
	kc.dispatcher.Start()

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

		kc.dispatcher.Dispatch(worker.GiftTask{
			GiftName:   giftName,
			Count:      count,
			KsNickname: userName,
			KsAvatar:   userAvatar,
		})
	}

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

		if kc.chatTrigger != "" && content == kc.chatTrigger {
			virtualName := kc.pickRandomChatAction()
			log.Printf("[弹幕触发] %s 发送 %s → %s", userName, content, virtualName)
			kc.dispatcher.Dispatch(worker.GiftTask{
				GiftName:   virtualName,
				Count:      1,
				KsNickname: userName,
				KsAvatar:   userAvatar,
			})
		}
	}
}
