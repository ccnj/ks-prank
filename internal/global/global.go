package global

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-resty/resty/v2"

	"ks-prank/config"
)

var (
	HttpClient *resty.Client
	MQTTClient mqtt.Client
	Config     *config.Config
)
