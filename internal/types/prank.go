package types

import "encoding/json"

// ActionChoice 一个触发器下可选的动作之一（按权重随机）
type ActionChoice struct {
	Action      string          `json:"action"`
	Weight      int             `json:"weight"`
	WorkerGroup int             `json:"worker_group"`
	Params      json.RawMessage `json:"params"`
}

type GiftTrigger struct {
	GiftName string         `json:"gift_name"`
	Choices  []ActionChoice `json:"choices"`
}

type ChatTrigger struct {
	Keyword string         `json:"keyword"`
	Choices []ActionChoice `json:"choices"`
}

type LikeTrigger struct {
	Threshold uint64         `json:"threshold"`
	Choices   []ActionChoice `json:"choices"`
}

type PrankConfigData struct {
	GiftTriggers []GiftTrigger `json:"gift_triggers"`
	ChatTriggers []ChatTrigger `json:"chat_triggers"`
	LikeTrigger  *LikeTrigger  `json:"like_trigger,omitempty"`
}

// LiveAccount 从 server 拉取的直播账号
type LiveAccount struct {
	Id       string `json:"id"`
	SiteId   string `json:"site_id"`
	Platform string `json:"platform"`
	LiveUrl  string `json:"live_url"`
	Nickname string `json:"nickname"`
	Enabled  bool   `json:"enabled"`
}

type SiteSimple struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ArBox struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type ProfileUser struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

type Profile struct {
	User         ProfileUser    `json:"user"`
	Site         *SiteSimple    `json:"site"`
	ArBoxes      []ArBox        `json:"ar_boxes"`
	LiveAccounts []*LiveAccount `json:"live_accounts"`
}

// RuntimeConfig 连接期间的完整业务上下文（不持久化）
type RuntimeConfig struct {
	UserId   string
	SiteId   string
	ArBoxId  string
	LiveUrl  string
	Platform string
	Prank    *PrankConfigData
}
