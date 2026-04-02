package initialize

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	glb "ks-prank/internal/global"
)

// MqttConfig 从服务端获取的 MQTT 连接配置
type MqttConfig struct {
	Broker   string
	Username string
	Password string
}

func InitMqtt(mqttCfg *MqttConfig) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttCfg.Broker)
	opts.SetClientID(fmt.Sprintf("ks-prank-%d", rand.Intn(100000)))
	opts.SetUsername(mqttCfg.Username)
	opts.SetPassword(mqttCfg.Password)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("[MQTT] 连接断开: %v", err)
	})
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("[MQTT] 已连接")
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("MQTT 连接超时")
	}
	if token.Error() != nil {
		return fmt.Errorf("MQTT 连接失败: %w", token.Error())
	}

	glb.MQTTClient = client
	log.Println("[MQTT] 初始化完成")
	return nil
}

// FetchMqttConfig 从 luck-pets-server 获取 MQTT 连接配置
func FetchMqttConfig() (*MqttConfig, error) {
	if glb.HttpClient == nil {
		return nil, fmt.Errorf("http client 未初始化")
	}

	reqBody := map[string]interface{}{
		"role":    "KS_PRANK",
		"sec_key": "luckpets@fight#2026",
	}

	var rsp struct {
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
		Data    struct {
			Broker   string `json:"broker"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"data"`
	}

	resp, err := glb.HttpClient.R().
		SetBody(reqBody).
		SetResult(&rsp).
		Post("/api/v1/fight/low_security/get_mqtt_config")
	if err != nil {
		return nil, fmt.Errorf("请求 MQTT 配置失败: %w", err)
	}
	if !resp.IsSuccess() || rsp.ErrCode != 0 {
		return nil, fmt.Errorf("获取 MQTT 配置失败: status=%d errCode=%d errMsg=%s", resp.StatusCode(), rsp.ErrCode, rsp.ErrMsg)
	}

	return &MqttConfig{
		Broker:   rsp.Data.Broker,
		Username: rsp.Data.Username,
		Password: rsp.Data.Password,
	}, nil
}
