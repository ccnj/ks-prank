package global

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-resty/resty/v2"

	"ks-prank/config"
	mytypes "ks-prank/internal/types"
)

var (
	HttpClient *resty.Client
	MQTTClient mqtt.Client
	Config     *config.Config
	// Runtime 当前连接会话的业务配置（来自服务端）
	Runtime *mytypes.RuntimeConfig
)
