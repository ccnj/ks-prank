package handler

import (
	"fmt"
	"time"

	"ks-prank/config"
	"ks-prank/internal/consts"
	glb "ks-prank/internal/global"
)

const (
	healMonsterPath  = "/api/v1/fight/low_security/heal_monster"
	healMonsterHp    = 10000
	healMonsterText  = "为主播恢复血量 x%d"
)

type healMonsterResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

func HealMonster(nickname, avatar string, giftCount int) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	giftCount = normalizeGiftCount(giftCount)

	if err := publishLiveRoomGiftInfo(nickname, avatar, fmt.Sprintf(healMonsterText, giftCount), importanceLow); err != nil {
		return err
	}

	for i := 0; i < giftCount; i++ {
		reqBody := map[string]interface{}{
			"ar_box_id": config.ConfIns.ArBoxId,
			"heal_hp":   healMonsterHp,
			"sec_key":   consts.LowSecurityKey,
		}

		var rsp healMonsterResponse
		resp, err := glb.HttpClient.R().
			SetBody(reqBody).
			SetResult(&rsp).
			Post(healMonsterPath)
		if err != nil {
			return fmt.Errorf("调用heal_monster失败 index=%d: %w", i+1, err)
		}
		if !resp.IsSuccess() || rsp.ErrCode != 0 {
			return fmt.Errorf("调用heal_monster失败 index=%d status=%d errCode=%d errMsg=%s", i+1, resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil
}
