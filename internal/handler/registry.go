package handler

import (
	"encoding/json"
	"fmt"
	"log"

	mytypes "ks-prank/internal/types"
)

// HandlerCtx 调用 handler 时的触发上下文
type HandlerCtx struct {
	Nickname  string
	Avatar    string
	GiftCount int
}

// HandlerFn 每个 action 对应的执行函数；负责 unmarshal 自己关心的 params
type HandlerFn func(ctx HandlerCtx, params json.RawMessage) error

// Handlers 新增 action 时注册到此处；params 里的字段由本 handler 自行解析
var Handlers = map[string]HandlerFn{
	"attack_monster_360": func(ctx HandlerCtx, params json.RawMessage) error {
		var p struct {
			ShootCnt   int `json:"shoot_cnt"`
			HitLevel   int `json:"hit_level"`
			Importance int `json:"importance"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return fmt.Errorf("解析 attack_monster_360 参数失败: %w", err)
			}
		}
		log.Printf("执行 attack_monster_360: shoot=%d level=%d (%s)", p.ShootCnt, p.HitLevel, hitLevelSkillName(p.HitLevel))
		return AttackMonster360(ctx.Nickname, ctx.Avatar, ctx.GiftCount, p.ShootCnt, p.HitLevel, p.Importance)
	},

	"heal_monster": func(ctx HandlerCtx, params json.RawMessage) error {
		var p struct {
			Importance int `json:"importance"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return fmt.Errorf("解析 heal_monster 参数失败: %w", err)
			}
		}
		log.Printf("执行 heal_monster")
		return HealMonster(ctx.Nickname, ctx.Avatar, ctx.GiftCount, p.Importance)
	},

	"throw_cockroach": func(ctx HandlerCtx, params json.RawMessage) error {
		var p struct {
			Count      int `json:"count"`
			Importance int `json:"importance"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return fmt.Errorf("解析 throw_cockroach 参数失败: %w", err)
			}
		}
		if p.Count < 1 {
			p.Count = 1
		}
		log.Printf("执行 throw_cockroach: count=%d", p.Count)
		return ThrowCockroach(ctx.Nickname, ctx.Avatar, ctx.GiftCount*p.Count, p.Importance)
	},
}

// RunChoice 按 choice 指定的 action 执行
func RunChoice(ctx HandlerCtx, c mytypes.ActionChoice) error {
	fn, ok := Handlers[c.Action]
	if !ok {
		return fmt.Errorf("未知 action: %s", c.Action)
	}
	return fn(ctx, c.Params)
}
