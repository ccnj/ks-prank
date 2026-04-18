package handler

import (
	"encoding/json"
	"fmt"
	"time"

	glb "ks-prank/internal/global"
)

const (
	spinMsgType        = "MOVE"
	spinRole           = "KS_PRANK"
	spinIdentity       = "ks-prank"
	spinSpeedLeft      = -70
	spinSpeedRight     = 70
	spinBurstDuration  = 500 * time.Millisecond
	spinTickInterval   = 50 * time.Millisecond
	spinTextTpl        = "原地旋转 x%d"
	getUsingSnByUidPth = "/api/v1/fight/low_security/get_using_sn_by_uid"
)

type spinMessage struct {
	Header  spinHeader  `json:"header"`
	Payload spinPayload `json:"payload"`
}

type spinHeader struct {
	MsgID    string `json:"msg_id"`
	Ts       int64  `json:"ts"`
	Type     string `json:"type"`
	Role     string `json:"role"`
	Identity string `json:"identity"`
	Identify string `json:"identify,omitempty"`
}

type spinPayload struct {
	LSpeed int `json:"l_speed"`
	RSpeed int `json:"r_speed"`
}

type getUsingSnByUidResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    struct {
		Sn string `json:"sn"`
	} `json:"data"`
}

// Spin 向主播在用的遥控车发送旋转控制
func Spin(nickname, avatar string, giftCount, importance int) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	if glb.Runtime == nil || glb.Runtime.UserId == "" {
		return fmt.Errorf("未登录，跳过旋转动作")
	}
	giftCount = normalizeGiftCount(giftCount)

	sn, err := getUsingSnByUID(glb.Runtime.UserId)
	if err != nil {
		return err
	}
	if sn == "" {
		return fmt.Errorf("未查询到主播在用的遥控车")
	}

	if err := publishLiveRoomGiftInfo(nickname, avatar, fmt.Sprintf(spinTextTpl, giftCount), importance); err != nil {
		return err
	}

	topic := fmt.Sprintf("RC/%s/ctrl", sn)
	for i := 0; i < giftCount; i++ {
		sendCount := int(spinBurstDuration / spinTickInterval)
		for tick := 0; tick < sendCount; tick++ {
			msg := spinMessage{
				Header: spinHeader{
					MsgID:    fmt.Sprintf("%d_spin_%d_%d", time.Now().UnixMilli(), i, tick),
					Ts:       time.Now().UnixMilli(),
					Type:     spinMsgType,
					Role:     spinRole,
					Identity: spinIdentity,
					Identify: spinIdentity,
				},
				Payload: spinPayload{LSpeed: spinSpeedLeft, RSpeed: spinSpeedRight},
			}

			body, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("序列化旋转消息失败: %w", err)
			}
			token := glb.MQTTClient.Publish(topic, 1, false, body)
			if !token.WaitTimeout(5 * time.Second) {
				return fmt.Errorf("发布旋转消息超时 index=%d tick=%d", i+1, tick+1)
			}
			if token.Error() != nil {
				return fmt.Errorf("发布旋转消息失败 index=%d tick=%d: %w", i+1, tick+1, token.Error())
			}
			if tick < sendCount-1 {
				time.Sleep(spinTickInterval)
			}
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func getUsingSnByUID(uid string) (string, error) {
	reqBody := map[string]interface{}{
		"uid":     uid,
		"sec_key": lowSecurityKey,
	}

	var rsp getUsingSnByUidResponse
	resp, err := glb.HttpClient.R().
		SetBody(reqBody).
		SetResult(&rsp).
		Post(getUsingSnByUidPth)
	if err != nil {
		return "", fmt.Errorf("查询设备 SN 失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		return "", fmt.Errorf("查询设备 SN 失败: status=%d errCode=%d errMsg=%s",
			resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}
	return rsp.Data.Sn, nil
}
