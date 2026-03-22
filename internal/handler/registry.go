package handler

import (
	"log"

	"ks-prank/config"
	"ks-prank/internal/worker"
)

type ActionFactory func(cfg config.GiftActionConfig) worker.GiftAction

var ActionRegistry = map[string]ActionFactory{
	"attack_monster_360": func(cfg config.GiftActionConfig) worker.GiftAction {
		shootCnt := cfg.Params.ShootCnt
		hitLevel := cfg.Params.HitLevel
		return func(task worker.GiftTask) {
			log.Printf("执行 %s: %s", cfg.GiftName, hitLevelSkillName(hitLevel))
			if err := AttackMonster360(task.KsNickname, task.KsAvatar, task.Count, shootCnt, hitLevel); err != nil {
				log.Printf("攻击失败: %v", err)
			}
		}
	},
	"heal_monster": func(cfg config.GiftActionConfig) worker.GiftAction {
		return func(task worker.GiftTask) {
			log.Printf("执行 %s: 回血", cfg.GiftName)
			if err := HealMonster(task.KsNickname, task.KsAvatar, task.Count); err != nil {
				log.Printf("回血失败: %v", err)
			}
		}
	},
}
