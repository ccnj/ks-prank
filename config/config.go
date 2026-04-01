package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	// 服务端配置
	ServerURL    string `mapstructure:"server_url"`
	MqttBroker   string `mapstructure:"mqtt_broker"`
	MqttUsername string `mapstructure:"mqtt_username"`
	MqttPassword string `mapstructure:"mqtt_password"`

	// 业务配置
	ArBoxId string `mapstructure:"ar_box_id"`
	SiteId  string `mapstructure:"site_id"`

	// 快手连接
	LiveUrl      string `mapstructure:"live_url"`
	WssUrl       string `mapstructure:"wss_url"`
	Token        string `mapstructure:"token"`
	LiveStreamId string `mapstructure:"live_stream_id"`

	// 触发配置
	GiftActions []GiftActionConfig `mapstructure:"gift_actions"`
	ChatAction  *ChatActionConfig  `mapstructure:"chat_action"`
}

type GiftActionConfig struct {
	GiftName    string       `mapstructure:"gift_name"`
	Action      string       `mapstructure:"action"`
	WorkerGroup int          `mapstructure:"worker_group"`
	Params      ActionParams `mapstructure:"params"`
}

type ActionParams struct {
	ShootCnt   int `mapstructure:"shoot_cnt"`
	HitLevel   int `mapstructure:"hit_level"`
	Importance int `mapstructure:"importance"`
}

type ChatActionConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	Trigger string            `mapstructure:"trigger"`
	Actions []ChatActionEntry `mapstructure:"actions"`
}

type ChatActionEntry struct {
	Action      string       `mapstructure:"action"`
	Weight      int          `mapstructure:"weight"`
	WorkerGroup int          `mapstructure:"worker_group"`
	Params      ActionParams `mapstructure:"params"`
}

var ConfIns *Config

func init() {
	v := viper.New()
	v.SetConfigFile("config.yaml")
	if err := v.ReadInConfig(); err != nil {
		panic(err)
	}
	if err := v.Unmarshal(&ConfIns); err != nil {
		panic(err)
	}
	log.Printf("配置加载完成: ar_box_id=%s site_id=%s", ConfIns.ArBoxId, ConfIns.SiteId)
}
