package service

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ks-prank/internal/handler"
	mytypes "ks-prank/internal/types"
	"ks-prank/internal/worker"
	"ks-prank/pkg/douyincrawler"
	pb "ks-prank/pkg/douyincrawler/proto"
)

const sendToAnchorMark = ":送给主播 "

// DouyinPrankClient 抖音直播间整蛊客户端（crawler + 触发器分发）
type DouyinPrankClient struct {
	crawler    *douyincrawler.DouyinClient
	dispatcher *worker.Dispatcher
	deduper    *giftDeduper
	eventCb    EventCallback

	prank *mytypes.PrankConfigData

	giftTriggerMap map[string]*mytypes.GiftTrigger
	chatTriggerMap map[string]*mytypes.ChatTrigger

	// 点赞按用户累计：达到阈值触发
	likeMu         sync.Mutex
	likePerUser    map[string]uint64
	likeThreshold  uint64
	startedOnce    sync.Once
	stopOnce       sync.Once
	closed         atomic.Bool
}

// NewDouyinPrankClient 构造抖音端客户端
func NewDouyinPrankClient(wssURL string, prank *mytypes.PrankConfigData, eventCb EventCallback) (*DouyinPrankClient, error) {
	kc := &DouyinPrankClient{
		dispatcher:     worker.NewDispatcher(200),
		deduper:        newGiftDeduper(3 * time.Minute),
		eventCb:        eventCb,
		prank:          prank,
		giftTriggerMap: make(map[string]*mytypes.GiftTrigger),
		chatTriggerMap: make(map[string]*mytypes.ChatTrigger),
		likePerUser:    make(map[string]uint64),
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
		if prank.LikeTrigger != nil {
			kc.likeThreshold = prank.LikeTrigger.Threshold
		}
	}

	cfg := douyincrawler.MessageHandlerConfig{
		GiftHandler:   &dyGiftBridge{owner: kc},
		ChatHandler:   &dyChatBridge{owner: kc},
		MemberHandler: &dyMemberBridge{},
		LikeHandler:   &dyLikeBridge{owner: kc},
	}
	crawler, err := douyincrawler.NewDouyinClient(wssURL, cfg)
	if err != nil {
		return nil, err
	}
	kc.crawler = crawler
	return kc, nil
}

func (kc *DouyinPrankClient) Listen() {
	kc.startedOnce.Do(func() {
		if err := kc.crawler.Start(); err != nil {
			log.Printf("抖音 crawler 退出: %v", err)
		}
	})
}

func (kc *DouyinPrankClient) Close() {
	kc.stopOnce.Do(func() {
		kc.closed.Store(true)
		_ = kc.crawler.Shutdown()
		kc.dispatcher.Stop()
		log.Println("抖音客户端已关闭")
	})
}

func (kc *DouyinPrankClient) emitEvent(eventType EventType, data any) {
	if kc.eventCb == nil {
		return
	}
	kc.eventCb(EventPayload{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	})
}

func (kc *DouyinPrankClient) dispatchChoice(name string, hctx handler.HandlerCtx, choice *mytypes.ActionChoice) {
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

// ===== 消息桥 =====

type dyGiftBridge struct{ owner *DouyinPrankClient }
type dyChatBridge struct{ owner *DouyinPrankClient }
type dyMemberBridge struct{}
type dyLikeBridge struct{ owner *DouyinPrankClient }

func (b *dyGiftBridge) HandleGift(user *pb.User, giftInfo *pb.GiftMessage) {
	b.owner.handleGift(user, giftInfo)
}
func (b *dyChatBridge) HandleChat(user *pb.User, chatInfo *pb.ChatMessage) {
	b.owner.handleChat(user, chatInfo)
}
func (*dyMemberBridge) HandleMember(user *pb.User, memberInfo *pb.MemberMessage) {}
func (b *dyLikeBridge) HandleLike(user *pb.User, likeInfo *pb.LikeMessage) {
	b.owner.handleLike(user, likeInfo)
}

// ===== 消息处理 =====

func (kc *DouyinPrankClient) handleGift(user *pb.User, giftInfo *pb.GiftMessage) {
	if giftInfo == nil || giftInfo.Common == nil {
		return
	}
	if !kc.deduper.MarkIfNew(buildDyDedupKey(user, giftInfo)) {
		return
	}

	describe := strings.TrimSpace(giftInfo.Common.Describe)
	if !strings.Contains(describe, sendToAnchorMark) {
		if strings.Contains(describe, ":送给") {
			return
		}
		log.Printf("礼物消息格式异常: %s", describe)
		return
	}
	parts := strings.SplitN(describe, sendToAnchorMark, 2)
	if len(parts) < 2 {
		log.Printf("礼物消息格式异常: %s", describe)
		return
	}
	userName := parts[0]
	giftParts := strings.SplitN(parts[1], "个", 2)
	if len(giftParts) < 2 {
		log.Printf("礼物消息格式异常: %s", describe)
		return
	}
	count, err := strconv.Atoi(strings.TrimSpace(giftParts[0]))
	if err != nil {
		log.Printf("礼物数量转换失败: %v", err)
		return
	}
	giftName := strings.TrimSpace(giftParts[1])

	// 单发为 count=1&RepeatEnd=0；连击过程中 count 累计、RepeatEnd=0；结束时 RepeatEnd=1
	if !((count == 1 && giftInfo.RepeatEnd != 1) ||
		(giftInfo.RepeatEnd == 1 && count != 1)) {
		return
	}
	if giftInfo.RepeatEnd == 1 && count != 1 {
		count = count - 1
	}

	giftValue := 0
	if giftInfo.Gift != nil {
		giftValue = int(giftInfo.Gift.GetDiamondCount())
	}
	avatar := getDyAvatarURL(user)
	dyUid := getDyUID(user)

	log.Printf("[抖音礼物] %s 送出 %s x%d (%d钻)", userName, giftName, count, giftValue)
	go handler.ReportDyGiftLog(dyUid, giftName, count, giftValue, giftInfo)

	kc.emitEvent(EventGift, GiftEventData{
		Username: userName,
		Avatar:   avatar,
		GiftName: giftName,
		Price:    giftValue,
		Count:    count,
	})

	if trigger, ok := kc.giftTriggerMap[giftName]; ok {
		choice := pickChoice(trigger.Choices)
		kc.dispatchChoice(giftName, handler.HandlerCtx{
			Nickname:  userName,
			Avatar:    avatar,
			GiftCount: count,
		}, choice)
	}
}

func (kc *DouyinPrankClient) handleChat(user *pb.User, chatInfo *pb.ChatMessage) {
	if chatInfo == nil || user == nil {
		return
	}
	content := strings.TrimSpace(chatInfo.Content)
	userName := user.GetNickName()
	avatar := getDyAvatarURL(user)

	log.Printf("[抖音弹幕] %s: %s", userName, content)
	kc.emitEvent(EventComment, CommentEventData{
		Username: userName,
		Avatar:   avatar,
		Content:  content,
	})

	if trigger, ok := kc.chatTriggerMap[content]; ok {
		choice := pickChoice(trigger.Choices)
		kc.dispatchChoice("chat:"+content, handler.HandlerCtx{
			Nickname:  userName,
			Avatar:    avatar,
			GiftCount: 1,
		}, choice)
	}
}

func (kc *DouyinPrankClient) handleLike(user *pb.User, likeInfo *pb.LikeMessage) {
	if kc.likeThreshold == 0 || kc.prank == nil || kc.prank.LikeTrigger == nil {
		return
	}
	if user == nil || likeInfo.GetCount() == 0 {
		return
	}
	uid := getDyUID(user)
	if uid == "" {
		return
	}

	count := likeInfo.GetCount()
	kc.likeMu.Lock()
	oldTotal := kc.likePerUser[uid]
	newTotal := oldTotal + count
	kc.likePerUser[uid] = newTotal
	kc.likeMu.Unlock()

	oldBucket := oldTotal / kc.likeThreshold
	newBucket := newTotal / kc.likeThreshold
	if newBucket <= oldBucket {
		return
	}

	triggerCount := int(newBucket - oldBucket)
	avatar := getDyAvatarURL(user)
	userName := user.GetNickName()
	log.Printf("[抖音点赞] %s 累计 %d，触发惩罚 x%d", userName, newTotal, triggerCount)

	choice := pickChoice(kc.prank.LikeTrigger.Choices)
	for i := 0; i < triggerCount; i++ {
		kc.dispatchChoice("like_threshold", handler.HandlerCtx{
			Nickname:  userName,
			Avatar:    avatar,
			GiftCount: 1,
		}, choice)
	}
}

// ===== 工具函数 =====

func getDyAvatarURL(user *pb.User) string {
	if user == nil || user.AvatarThumb == nil {
		return ""
	}
	urls := user.AvatarThumb.GetUrlListList()
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func getDyUID(user *pb.User) string {
	if user == nil {
		return ""
	}
	if user.IdStr != "" {
		return user.IdStr
	}
	return strconv.FormatUint(user.Id, 10)
}

func buildDyDedupKey(user *pb.User, giftInfo *pb.GiftMessage) string {
	if giftInfo == nil {
		return ""
	}
	common := giftInfo.GetCommon()
	dyUID := getDyUID(user)

	if common != nil {
		if logID := common.GetLogId(); logID != "" {
			return "common_logid:" + logID
		}
		if msgID := common.GetMsgId(); msgID != 0 {
			return fmt.Sprintf("common_msgid:%d|uid:%s|gift:%d", msgID, dyUID, giftInfo.GetGiftId())
		}
	}
	if logID := giftInfo.GetLogId(); logID != "" {
		return "gift_logid:" + logID
	}
	var createTime uint64
	var describe string
	if common != nil {
		createTime = common.GetCreateTime()
		describe = strings.TrimSpace(common.GetDescribe())
	}
	return fmt.Sprintf("fallback|uid:%s|gift:%d|repeat:%d|create:%d|desc:%s",
		dyUID, giftInfo.GetGiftId(), giftInfo.GetRepeatEnd(), createTime, describe)
}

// pickChoice 在 kuaishou.go 中定义，此处不再重复
