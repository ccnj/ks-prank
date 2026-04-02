package initialize

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"ks-prank/config"
	glb "ks-prank/internal/global"
)

func InitMqtt(cfg *config.Config) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MqttBroker)
	opts.SetClientID("ks-prank")
	opts.SetUsername(cfg.MqttUsername)
	opts.SetPassword(cfg.MqttPassword)
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
