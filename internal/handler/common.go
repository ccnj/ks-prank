package handler

import (
	"encoding/json"
	"fmt"
	"time"

	glb "ks-prank/internal/global"
)

const (
	lowSecurityKey = "luckpets@fight#2026"

	liveRoomGiftTopicTpl = "SITE/%s/live_room_gift"
	liveRoomGiftMsgType  = "LIVE_ROOM_GIFT"
	liveRoomGiftRole     = "KS_PRANK"
	liveRoomGiftIdentity = "ks-prank"

	importanceLow    = 0
	importanceNormal = 1
)

type liveRoomGiftMessage struct {
	Header  msgHeader           `json:"header"`
	Payload liveRoomGiftPayload `json:"payload"`
}

type msgHeader struct {
	MsgID    string `json:"msg_id"`
	Ts       int64  `json:"ts"`
	Type     string `json:"type"`
	Role     string `json:"role"`
	Identity string `json:"identity"`
}

type liveRoomGiftPayload struct {
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	Text       string `json:"text"`
	Importance int    `json:"importance"`
}

const (
	addLiveGiftLogPath = "/api/v1/fight/low_security/add_live_gift_log"

	PlatformKs = "ks"
	PlatformDy = "dy"
)

type addGiftLogResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

// ReportLiveGiftLog 上报直播间礼物日志（合并 ks/dy）。
// platform 取值 PlatformKs / PlatformDy；platformUid 为送礼用户在该平台的 UID。
// anchor_uid / site_id / live_url 从 glb.Runtime 读取。
func ReportLiveGiftLog(platform, platformUid, giftName string, count, giftValue int, rawInfo interface{}) {
	if glb.HttpClient == nil {
		return
	}

	rt := glb.Runtime
	if rt == nil {
		fmt.Printf("记录直播间礼物日志失败: runtime 未就绪\n")
		return
	}

	reqBody := map[string]interface{}{
		"anchor_uid":   rt.UserId,
		"site_id":      rt.SiteId,
		"live_url":     rt.LiveUrl,
		"platform":     platform,
		"platform_uid": platformUid,
		"gift_name":    giftName,
		"count":        count,
		"gift_value":   giftValue,
		"raw_info":     rawInfo,
		"sec_key":      lowSecurityKey,
	}

	var rsp addGiftLogResponse
	resp, err := glb.HttpClient.R().
		SetBody(reqBody).
		SetResult(&rsp).
		Post(addLiveGiftLogPath)
	if err != nil {
		fmt.Printf("记录直播间礼物日志失败(%s): %v\n", platform, err)
		return
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		fmt.Printf("记录直播间礼物日志失败(%s): status=%d errCode=%d errMsg=%s\n",
			platform, resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}
}

func ensureClientsReady() error {
	if glb.HttpClient == nil {
		return fmt.Errorf("http client 未初始化")
	}
	if glb.MQTTClient == nil {
		return fmt.Errorf("mqtt client 未初始化")
	}
	return nil
}

func normalizeGiftCount(giftCount int) int {
	if giftCount <= 0 {
		return 1
	}
	return giftCount
}

func publishLiveRoomGiftInfo(nickname, avatar, text string, importance int) error {
	msg := liveRoomGiftMessage{
		Header: msgHeader{
			MsgID:    fmt.Sprintf("%d_live_room_gift", time.Now().UnixMilli()),
			Ts:       time.Now().UnixMilli(),
			Type:     liveRoomGiftMsgType,
			Role:     liveRoomGiftRole,
			Identity: liveRoomGiftIdentity,
		},
		Payload: liveRoomGiftPayload{
			Nickname:   nickname,
			Avatar:     avatar,
			Text:       text,
			Importance: importance,
		},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化通知消息失败: %w", err)
	}

	topic := fmt.Sprintf(liveRoomGiftTopicTpl, glb.Runtime.SiteId)
	token := glb.MQTTClient.Publish(topic, 1, false, body)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("发布通知消息超时, topic: %s", topic)
	}
	if token.Error() != nil {
		return fmt.Errorf("发布通知消息失败: %w", token.Error())
	}
	return nil
}
