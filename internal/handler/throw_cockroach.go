package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"ks-prank/config"
	glb "ks-prank/internal/global"
)

const (
	prankEventTopicTpl = "SITE/%s/prank_event"
	prankEventRole     = "KS_PRANK"
	prankEventIdentity = "ks-prank"
)

type prankEventMessage struct {
	Header  msgHeader         `json:"header"`
	Payload prankEventPayload `json:"payload"`
}

type prankEventPayload struct {
	Count int `json:"count"`
}

func ThrowCockroach(nickname, avatar string, giftCount, importance int) error {
	if glb.MQTTClient == nil {
		return fmt.Errorf("mqtt client 未初始化")
	}
	giftCount = normalizeGiftCount(giftCount)

	text := fmt.Sprintf("丢蟑螂 x%d", giftCount)
	if err := publishLiveRoomGiftInfo(nickname, avatar, text, importance); err != nil {
		return err
	}

	msg := prankEventMessage{
		Header: msgHeader{
			MsgID:    fmt.Sprintf("%d_throw_cockroach", time.Now().UnixMilli()),
			Ts:       time.Now().UnixMilli(),
			Type:     "THROW_COCKROACH",
			Role:     prankEventRole,
			Identity: prankEventIdentity,
		},
		Payload: prankEventPayload{
			Count: giftCount,
		},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化整蛊消息失败: %w", err)
	}

	topic := fmt.Sprintf(prankEventTopicTpl, config.ConfIns.SiteId)
	token := glb.MQTTClient.Publish(topic, 1, false, body)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("发布整蛊消息超时, topic: %s", topic)
	}
	if token.Error() != nil {
		return fmt.Errorf("发布整蛊消息失败: %w", token.Error())
	}
	return nil
}
