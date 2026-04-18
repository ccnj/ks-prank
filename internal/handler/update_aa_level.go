package handler

import (
	"fmt"
	"time"

	glb "ks-prank/internal/global"
)

const (
	updateAaLevelPath    = "/api/v1/fight/low_security/update_user_aa_level"
	updateAaLevelUpTpl   = "升级武器 x%d"
	updateAaLevelDownTpl = "降级武器 x%d"
)

type updateAaLevelResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

// UpdateUserAaLevel 调整主播武器等级（levelDelta 仅支持 +1/-1）
func UpdateUserAaLevel(nickname, avatar string, giftCount, levelDelta, importance int) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	if glb.Runtime == nil || glb.Runtime.UserId == "" {
		return fmt.Errorf("未登录，跳过武器等级调整")
	}
	if levelDelta != 1 && levelDelta != -1 {
		return fmt.Errorf("level_delta 仅支持 1 或 -1")
	}
	giftCount = normalizeGiftCount(giftCount)

	text := fmt.Sprintf(updateAaLevelUpTpl, giftCount)
	if levelDelta < 0 {
		text = fmt.Sprintf(updateAaLevelDownTpl, giftCount)
	}
	if err := publishLiveRoomGiftInfo(nickname, avatar, text, importance); err != nil {
		return err
	}

	for i := 0; i < giftCount; i++ {
		reqBody := map[string]interface{}{
			"uid":         glb.Runtime.UserId,
			"level_delta": levelDelta,
			"sec_key":     lowSecurityKey,
		}

		var rsp updateAaLevelResponse
		resp, err := glb.HttpClient.R().
			SetBody(reqBody).
			SetResult(&rsp).
			Post(updateAaLevelPath)
		if err != nil {
			return fmt.Errorf("调用 update_user_aa_level 失败 index=%d: %w", i+1, err)
		}
		if !resp.IsSuccess() || rsp.ErrCode != 0 {
			return fmt.Errorf("调用 update_user_aa_level 失败 index=%d status=%d errCode=%d errMsg=%s",
				i+1, resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
		}

		time.Sleep(500 * time.Millisecond)
	}
	return nil
}
