package handler

const petTeaseMsgType = "TEASE"

// PetTease 让整蛊车逗猫棒旋转 durationMs * giftCount 毫秒。
//
// 配置侧建议:
//   - worker_group 单独占一个号(例如 6), 与 pet_feed 不同 group 即可允许并发执行
//     (两套电机物理独立,可同时工作);同 group 让连击礼物自动排队。
//   - duration_ms 推荐 2000~5000ms。runPetHoldAction 会 clamp 到 [500, 10000]。
func PetTease(nickname, avatar string, giftCount, durationMs, importance int) error {
	return runPetHoldAction("撩猫", petTeaseMsgType, durationMs, giftCount, importance, nickname, avatar)
}
