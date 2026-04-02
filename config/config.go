package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	// 服务端配置
	ServerURL string `mapstructure:"server_url" json:"server_url" yaml:"server_url"`

	// 业务配置
	ArBoxId string `mapstructure:"ar_box_id" json:"ar_box_id" yaml:"ar_box_id"`
	SiteId  string `mapstructure:"site_id" json:"site_id" yaml:"site_id"`

	// 快手连接
	LiveUrl      string `mapstructure:"live_url" json:"live_url" yaml:"live_url"`
	WssUrl       string `mapstructure:"wss_url" json:"wss_url" yaml:"wss_url"`
	Token        string `mapstructure:"token" json:"token" yaml:"token"`
	LiveStreamId string `mapstructure:"live_stream_id" json:"live_stream_id" yaml:"live_stream_id"`

	// 触发配置
	GiftActions []GiftActionConfig `mapstructure:"gift_actions" json:"gift_actions" yaml:"gift_actions"`
	ChatAction  *ChatActionConfig  `mapstructure:"chat_action" json:"chat_action" yaml:"chat_action"`
}

type GiftActionConfig struct {
	GiftName    string       `mapstructure:"gift_name" json:"gift_name" yaml:"gift_name"`
	Action      string       `mapstructure:"action" json:"action" yaml:"action"`
	WorkerGroup int          `mapstructure:"worker_group" json:"worker_group" yaml:"worker_group"`
	Params      ActionParams `mapstructure:"params" json:"params" yaml:"params"`
}

type ActionParams struct {
	ShootCnt   int `mapstructure:"shoot_cnt" json:"shoot_cnt" yaml:"shoot_cnt"`
	HitLevel   int `mapstructure:"hit_level" json:"hit_level" yaml:"hit_level"`
	Importance int `mapstructure:"importance" json:"importance" yaml:"importance"`
}

type ChatActionConfig struct {
	Enabled bool              `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	Trigger string            `mapstructure:"trigger" json:"trigger" yaml:"trigger"`
	Actions []ChatActionEntry `mapstructure:"actions" json:"actions" yaml:"actions"`
}

type ChatActionEntry struct {
	Action      string       `mapstructure:"action" json:"action" yaml:"action"`
	Weight      int          `mapstructure:"weight" json:"weight" yaml:"weight"`
	WorkerGroup int          `mapstructure:"worker_group" json:"worker_group" yaml:"worker_group"`
	Params      ActionParams `mapstructure:"params" json:"params" yaml:"params"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
