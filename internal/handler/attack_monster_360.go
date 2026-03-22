package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ks-prank/config"
	"ks-prank/internal/consts"
	glb "ks-prank/internal/global"
)

const (
	getCurrentMonsterPath = "/api/v1/fight/low_security/get_current_monster"
	attackMsgType         = "VALID_SHOOT"
	attackRole            = "KS_PRANK"
	attackIdentity        = "ks-prank"
	attackHitType         = "AA"
)

func hitLevelSkillName(level int) string {
	switch level {
	case 1:
		return "小发雷霆"
	case 3:
		return "风之极-陨杀"
	default:
		return "致命一击"
	}
}

type fightMessage struct {
	Header  msgHeader    `json:"header"`
	Payload fightPayload `json:"payload"`
}

type fightPayload struct {
	ToMonsterID string `json:"to_monster_id"`
	Type        string `json:"type"`
	Level       int    `json:"level"`
	Angle       int    `json:"angle"`
}

type getCurrentMonsterResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    struct {
		MonsterID string `json:"monster_id"`
	} `json:"data"`
}

func AttackMonster360(nickname, avatar string, giftCount, shootCnt, hitLevel, importance int) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	giftCount = normalizeGiftCount(giftCount)

	fightTopic := fmt.Sprintf("BOX/%s/fight", config.ConfIns.ArBoxId)
	toMonsterID, err := getCurrentMonsterIDByTopic(fightTopic)
	if err != nil {
		return err
	}

	skillName := hitLevelSkillName(hitLevel)
	text := fmt.Sprintf("对主播释放了 %s x%d", skillName, giftCount)
	if err := publishLiveRoomGiftInfo(nickname, avatar, text, importance); err != nil {
		return err
	}

	step := 360 / (shootCnt - 1)
	for round := 0; round < giftCount; round++ {
		for i := 0; i < shootCnt; i++ {
			angle := i * step
			msg := fightMessage{
				Header: msgHeader{
					MsgID:    fmt.Sprintf("%d_%d_%d", time.Now().UnixMilli(), round, i),
					Ts:       time.Now().UnixMilli(),
					Type:     attackMsgType,
					Role:     attackRole,
					Identity: attackIdentity,
				},
				Payload: fightPayload{
					ToMonsterID: toMonsterID,
					Type:        attackHitType,
					Level:       hitLevel,
					Angle:       angle,
				},
			}

			body, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("序列化攻击消息失败: %w", err)
			}
			token := glb.MQTTClient.Publish(fightTopic, 1, false, body)
			if !token.WaitTimeout(5 * time.Second) {
				return fmt.Errorf("发布攻击消息超时 round=%d angle=%d", round+1, angle)
			}
			if token.Error() != nil {
				return fmt.Errorf("发布攻击消息失败 round=%d angle=%d: %w", round+1, angle, token.Error())
			}
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

func getCurrentMonsterIDByTopic(topic string) (string, error) {
	arBoxID, err := parseArBoxIDFromTopic(topic)
	if err != nil {
		return "", err
	}

	reqBody := map[string]interface{}{
		"ar_box_id": arBoxID,
		"sec_key":   consts.LowSecurityKey,
	}

	var rsp getCurrentMonsterResponse
	resp, err := glb.HttpClient.R().
		SetBody(reqBody).
		SetResult(&rsp).
		Post(getCurrentMonsterPath)
	if err != nil {
		return "", fmt.Errorf("获取当前怪兽失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		return "", fmt.Errorf("获取当前怪兽失败: status=%d errCode=%d errMsg=%s", resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}
	if rsp.Data.MonsterID == "" {
		return "", fmt.Errorf("当前没有可攻击目标")
	}

	return rsp.Data.MonsterID, nil
}

func parseArBoxIDFromTopic(topic string) (string, error) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("topic 格式不正确: %s", topic)
	}
	return parts[1], nil
}
