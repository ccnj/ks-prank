package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config 客户端持久化配置（只保留需要跨重启记住的少量字段）
// 业务配置（site/ar_box/gift_actions 等）从服务端拉取
type Config struct {
	ServerURL     string `mapstructure:"server_url" json:"server_url" yaml:"server_url"`
	AuthToken     string `mapstructure:"auth_token" json:"auth_token" yaml:"auth_token"`
	LastAccountId string `mapstructure:"last_account_id" json:"last_account_id" yaml:"last_account_id"`
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
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
