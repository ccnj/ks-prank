package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	glb "ks-prank/internal/global"
)

// 直接发往整蛊车 RC/{sn}/ctrl topic。需要 EMQX ACL 给 ks-prank 开通该 topic 的 publish 权限。
const (
	petCtrlTopicTpl = "RC/%s/ctrl"
	petCtrlRole     = "KS_PRANK"
	petCtrlIdentity = "ks-prank"

	// 按住式控制参数: 速度写死 80, 每 300ms 重发一次刷车端 600ms 保活
	petHoldSpeed   = 80
	petRepublishMs = 300

	// duration 安全 clamp, 防止配置错误把蠕动泵/逗猫棒长时间空转
	petDurationMin = 500
	petDurationMax = 10000
)

type petCtrlMessage struct {
	Header  msgHeader      `json:"header"`
	Payload petCtrlPayload `json:"payload"`
}

type petCtrlPayload struct {
	Speed int `json:"speed"`
}

// runPetHoldAction 让整蛊车上某种电机连续转 durationMs * giftCount 毫秒。
//
// 实现: 周期(每 300ms)往 RC/{sn}/ctrl 发 type=msgType speed=80 刷车端 600ms 保活;
// totalMs 到时再发一次 speed=0 立即停。函数阻塞直到时长跑完或出错。
//
// 该函数靠 ActionChoice.WorkerGroup 实现并发控制: 同 group 串行 → 同种动作连击礼物
// 自动排队;不同 group 并行 → 喂食与逗猫互不阻塞。具体 group 号由后台整蛊配置决定,
// handler 自身不感知。
func runPetHoldAction(label, msgType string, durationMs, giftCount, importance int, nickname, avatar string) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	if glb.Runtime == nil || glb.Runtime.PrankDeviceSn == "" {
		log.Printf("[%s] 该场地未配置整蛊车(prank_device_sn 为空),跳过", label)
		return nil
	}
	sn := glb.Runtime.PrankDeviceSn

	giftCount = normalizeGiftCount(giftCount)
	if durationMs < petDurationMin {
		durationMs = petDurationMin
	} else if durationMs > petDurationMax {
		durationMs = petDurationMax
	}
	totalMs := durationMs * giftCount

	text := fmt.Sprintf("%s x%d", label, giftCount)
	if err := publishLiveRoomGiftInfo(nickname, avatar, text, importance); err != nil {
		return err
	}

	topic := fmt.Sprintf(petCtrlTopicTpl, sn)

	// 立即发一次, 车端马上响应
	if err := publishPetCtrl(topic, msgType, petHoldSpeed); err != nil {
		return err
	}

	ticker := time.NewTicker(petRepublishMs * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(time.Duration(totalMs) * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			if err := publishPetCtrl(topic, msgType, petHoldSpeed); err != nil {
				// 单次发失败不致命, 继续 ramp 直到 timeout
				log.Printf("[%s] 周期发包失败: %v", label, err)
			}
		case <-timeout:
			// 收尾发一次 speed=0, 不依赖车端 600ms 保活就立即停
			return publishPetCtrl(topic, msgType, 0)
		}
	}
}

func publishPetCtrl(topic, msgType string, speed int) error {
	msg := petCtrlMessage{
		Header: msgHeader{
			MsgID:    fmt.Sprintf("%d_%s", time.Now().UnixMilli(), msgType),
			Ts:       time.Now().UnixMilli(),
			Type:     msgType,
			Role:     petCtrlRole,
			Identity: petCtrlIdentity,
		},
		Payload: petCtrlPayload{Speed: speed},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化 %s 消息失败: %w", msgType, err)
	}
	token := glb.MQTTClient.Publish(topic, 0, false, body)
	if !token.WaitTimeout(2 * time.Second) {
		return fmt.Errorf("发布 %s 消息超时, topic: %s", msgType, topic)
	}
	if token.Error() != nil {
		return fmt.Errorf("发布 %s 消息失败: %w", msgType, token.Error())
	}
	return nil
}
