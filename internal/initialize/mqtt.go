package initialize

import (
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"ks-prank/config"
	glb "ks-prank/internal/global"
)

func InitMqtt() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.ConfIns.MqttBroker)
	opts.SetClientID("ks-prank")
	opts.SetUsername(config.ConfIns.MqttUsername)
	opts.SetPassword(config.ConfIns.MqttPassword)
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
		log.Fatal("[MQTT] 连接超时")
	}
	if token.Error() != nil {
		log.Fatalf("[MQTT] 连接失败: %v", token.Error())
	}

	glb.MQTTClient = client
	log.Println("[MQTT] 初始化完成")
}
