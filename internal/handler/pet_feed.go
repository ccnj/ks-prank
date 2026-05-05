package handler

const petFeedMsgType = "FEED"

// PetFeed 让整蛊车蠕动泵挤肉泥 durationMs * giftCount 毫秒。
//
// 配置侧建议:
//   - worker_group 单独占一个号(例如 5), 不与攻击 / 回血共享;
//     同 group 让连击礼物自动排队,避免蠕动泵长时间不停转伤耗材。
//   - duration_ms 推荐 1000~3000ms。runPetHoldAction 会 clamp 到 [500, 10000]。
func PetFeed(nickname, avatar string, giftCount, durationMs, importance int) error {
	return runPetHoldAction("投喂", petFeedMsgType, durationMs, giftCount, importance, nickname, avatar)
}
