package handler

import (
	"fmt"
	"time"

	glb "ks-prank/internal/global"
)

const (
	addMonsterPath       = "/api/v1/fight/low_security/add_monster"
	addMonsterCreatorTpl = "ks-prank"
	addMonsterTextTpl    = "生成了年兽 x%d"
)

type addMonsterResponse struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
}

// AddMonster 按礼物数量调用后端生成年兽
func AddMonster(nickname, avatar string, giftCount int, monsterTplID string, importance int) error {
	if err := ensureClientsReady(); err != nil {
		return err
	}
	if glb.Runtime == nil || glb.Runtime.ArBoxId == "" {
		return fmt.Errorf("未绑定 MONSTER AR 盒子，跳过生成年兽")
	}
	if monsterTplID == "" {
		return fmt.Errorf("monster_tpl_id 不能为空")
	}
	giftCount = normalizeGiftCount(giftCount)

	creator := nickname
	if creator == "" {
		creator = addMonsterCreatorTpl
	}

	if err := publishLiveRoomGiftInfo(nickname, avatar, fmt.Sprintf(addMonsterTextTpl, giftCount), importance); err != nil {
		return err
	}

	for i := 0; i < giftCount; i++ {
		reqBody := map[string]interface{}{
			"monster_tpl_id": monsterTplID,
			"ar_box_id":      glb.Runtime.ArBoxId,
			"num":            1,
			"creator":        creator,
			"sec_key":        lowSecurityKey,
		}

		var rsp addMonsterResponse
		resp, err := glb.HttpClient.R().
			SetBody(reqBody).
			SetResult(&rsp).
			Post(addMonsterPath)
		if err != nil {
			return fmt.Errorf("调用 add_monster 失败 index=%d: %w", i+1, err)
		}
		if !resp.IsSuccess() || rsp.ErrCode != 0 {
			return fmt.Errorf("调用 add_monster 失败 index=%d status=%d errCode=%d errMsg=%s",
				i+1, resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
		}

		time.Sleep(500 * time.Millisecond)
	}
	return nil
}
