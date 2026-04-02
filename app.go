package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ks-prank/config"
	glb "ks-prank/internal/global"
	"ks-prank/internal/initialize"
	"ks-prank/internal/service"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var appConfigFile string
const baseConfigFile = "config.yaml"

func init() {
	configDir, _ := os.UserConfigDir()
	appConfigFile = filepath.Join(configDir, "ks-prank", "config-ks.yaml")
}

type App struct {
	ctx    context.Context
	mu     sync.Mutex
	client *service.KuaishouClient
	cfg    *config.Config
	status string // disconnected / connecting / connected / fetching_token
}

func NewApp() *App {
	return &App{status: "disconnected"}
}

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	// 首次启动：如果 config-ks.yaml 不存在，从 config.yaml 复制一份
	ensureAppConfig()

	cfg, err := config.LoadConfig(appConfigFile)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		cfg = &config.Config{
			WssUrl: "wss://livejs-ws-group5.gifshow.com/websocket",
		}
	}
	a.cfg = cfg
	glb.Config = cfg

	initialize.InitHttpClient(cfg.ServerURL)
}

func (a *App) OnShutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
}

// GetConfig 返回当前配置给前端
func (a *App) GetConfig() *config.Config {
	return a.cfg
}

// SaveConfig 保存前端传来的连接配置，保留 gift_actions/chat_action 等原有配置
func (a *App) SaveConfig(cfg config.Config) error {
	a.cfg.ServerURL = cfg.ServerURL
	a.cfg.ArBoxId = cfg.ArBoxId
	a.cfg.SiteId = cfg.SiteId
	a.cfg.LiveUrl = cfg.LiveUrl
	a.cfg.WssUrl = cfg.WssUrl
	a.cfg.Token = cfg.Token
	a.cfg.LiveStreamId = cfg.LiveStreamId

	glb.Config = a.cfg
	return config.SaveConfig(appConfigFile, a.cfg)
}

// GetStatus 返回当前连接状态
func (a *App) GetStatus() string {
	return a.status
}

// FetchToken 通过 Chrome 自动获取 WSS 信息
func (a *App) FetchToken(liveUrl string) (*initialize.WssInfo, error) {
	a.status = "fetching_token"
	runtime.EventsEmit(a.ctx, "event:status", "fetching_token")

	info, err := initialize.FetchWssInfo(liveUrl, 120*time.Second)
	if err != nil {
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		return nil, fmt.Errorf("获取 WSS 信息失败: %w", err)
	}

	a.cfg.Token = info.Token
	a.cfg.LiveStreamId = info.LiveStreamId
	if info.WssUrl != "" {
		a.cfg.WssUrl = info.WssUrl
	}

	a.status = "disconnected"
	runtime.EventsEmit(a.ctx, "event:status", "disconnected")
	return info, nil
}

// Connect 连接快手直播间
func (a *App) Connect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		return fmt.Errorf("已经连接中")
	}

	if a.cfg.Token == "" || a.cfg.LiveStreamId == "" {
		return fmt.Errorf("token 或 live_stream_id 为空")
	}

	a.status = "connecting"
	runtime.EventsEmit(a.ctx, "event:status", "connecting")

	initialize.InitHttpClient(a.cfg.ServerURL)

	mqttCfg, err := initialize.FetchMqttConfig()
	if err != nil {
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		return fmt.Errorf("获取 MQTT 配置失败: %w", err)
	}
	if err := initialize.InitMqtt(mqttCfg); err != nil {
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		return fmt.Errorf("MQTT 连接失败: %w", err)
	}

	eventCb := func(event service.EventPayload) {
		if event.Type == service.EventStatus {
			// status 事件前端期望接收字符串
			runtime.EventsEmit(a.ctx, "event:status", event.Data)
			return
		}
		runtime.EventsEmit(a.ctx, "event:"+string(event.Type), event)
	}

	client, err := service.NewKuaishouClient(a.cfg, eventCb)
	if err != nil {
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		return err
	}

	wssURL := a.cfg.WssUrl
	if wssURL == "" {
		wssURL = "wss://livejs-ws-group5.gifshow.com/websocket"
	}

	if err := client.Connect(wssURL, a.cfg.Token, a.cfg.LiveStreamId); err != nil {
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		return err
	}

	a.client = client
	a.status = "connected"
	runtime.EventsEmit(a.ctx, "event:status", "connected")

	go func() {
		client.Listen()
		a.mu.Lock()
		a.client = nil
		a.status = "disconnected"
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
	}()

	return nil
}

// Disconnect 断开连接
func (a *App) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("未连接")
	}

	a.client.Close()
	a.client = nil
	a.status = "disconnected"
	runtime.EventsEmit(a.ctx, "event:status", "disconnected")
	return nil
}

// ensureAppConfig 如果 ~/.ks-prank/config-ks.yaml 不存在，从 config.yaml 复制一份
func ensureAppConfig() {
	if _, err := os.Stat(appConfigFile); err == nil {
		return // 已存在
	}
	src, err := os.Open(baseConfigFile)
	if err != nil {
		return // config.yaml 也不存在，跳过
	}
	defer src.Close()

	os.MkdirAll(filepath.Dir(appConfigFile), 0755)
	dst, err := os.Create(appConfigFile)
	if err != nil {
		return
	}
	defer dst.Close()

	io.Copy(dst, src)
	log.Printf("已从 %s 创建 %s", baseConfigFile, appConfigFile)
}
