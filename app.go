package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ks-prank/config"
	glb "ks-prank/internal/global"
	"ks-prank/internal/initialize"
	"ks-prank/internal/service"
	mytypes "ks-prank/internal/types"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var appConfigFile string

const defaultServerURL = "https://mwapi.ybkc.cc"

func init() {
	configDir, _ := os.UserConfigDir()
	appConfigFile = filepath.Join(configDir, "ks-prank", "config-ks.yaml")
}

type App struct {
	ctx     context.Context
	mu      sync.Mutex
	client  service.PrankClient
	cfg     *config.Config
	profile *mytypes.Profile
	status  string // disconnected / connecting / connected / fetching_token
}

func NewApp() *App {
	return &App{status: "disconnected"}
}

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := config.LoadConfig(appConfigFile)
	if err != nil {
		log.Printf("加载配置失败: %v（使用默认）", err)
		cfg = &config.Config{ServerURL: defaultServerURL}
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL
	}
	a.cfg = cfg
	glb.Config = cfg

	initialize.InitHttpClient(cfg.ServerURL)
	if cfg.AuthToken != "" {
		service.SetAuthToken(cfg.AuthToken)
	}
}

func (a *App) OnShutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
}

func (a *App) persistConfig() {
	_ = os.MkdirAll(filepath.Dir(appConfigFile), 0755)
	if err := config.SaveConfig(appConfigFile, a.cfg); err != nil {
		log.Printf("保存配置失败: %v", err)
	}
}

// ===== 认证 =====

// LoginState 返回给前端用于判断登录态
type LoginState struct {
	LoggedIn bool   `json:"loggedIn"`
	Username string `json:"username,omitempty"`
}

func (a *App) GetLoginState() *LoginState {
	if a.cfg.AuthToken == "" {
		return &LoginState{LoggedIn: false}
	}
	// token 存在时尝试拉 profile 验证是否还有效，由前端通过 GetProfile 判断
	name := ""
	if a.profile != nil {
		name = a.profile.User.Username
	}
	return &LoginState{LoggedIn: true, Username: name}
}

func (a *App) Login(username, password string) error {
	initialize.InitHttpClient(a.cfg.ServerURL)
	token, err := service.AdminLogin(username, password)
	if err != nil {
		return err
	}
	a.cfg.AuthToken = token
	service.SetAuthToken(token)
	a.persistConfig()
	return nil
}

func (a *App) Logout() error {
	a.mu.Lock()
	if a.client != nil {
		a.client.Close()
		a.client = nil
		a.status = "disconnected"
		runtime.EventsEmit(a.ctx, "event:status", "disconnected")
	}
	a.mu.Unlock()

	a.cfg.AuthToken = ""
	a.cfg.LastAccountId = ""
	a.profile = nil
	service.SetAuthToken("")
	a.persistConfig()
	return nil
}

// GetProfile 主动刷新 profile（登录后调用）
func (a *App) GetProfile() (*mytypes.Profile, error) {
	if a.cfg.AuthToken == "" {
		return nil, fmt.Errorf("未登录")
	}
	p, err := service.GetProfile()
	if err != nil {
		return nil, err
	}
	a.profile = p
	return p, nil
}

// GetLastAccountId 记住上次用的直播账号
func (a *App) GetLastAccountId() string {
	return a.cfg.LastAccountId
}

// ===== 连接 =====

// GetStatus 返回当前连接状态
func (a *App) GetStatus() string {
	return a.status
}

// Connect 用指定的直播账号连接。需要先 Login + GetProfile 过。
func (a *App) Connect(liveAccountId string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		return fmt.Errorf("已经连接中，请先断开")
	}
	if a.profile == nil {
		return fmt.Errorf("请先加载 profile")
	}
	if a.profile.Site == nil {
		return fmt.Errorf("当前账号未绑定场地")
	}

	var account *mytypes.LiveAccount
	for _, acc := range a.profile.LiveAccounts {
		if acc.Id == liveAccountId {
			account = acc
			break
		}
	}
	if account == nil {
		return fmt.Errorf("未找到指定的直播账号")
	}
	if !account.Enabled {
		return fmt.Errorf("该直播账号已停用")
	}
	if account.LiveUrl == "" {
		return fmt.Errorf("直播账号 URL 为空")
	}

	// 自动挑第一个 MONSTER 类型的 AR 盒子（没有也允许连接，仅攻击/回血类动作会在运行时 no-op）
	var monsterBoxId string
	for _, b := range a.profile.ArBoxes {
		if b.Type == "MONSTER" {
			monsterBoxId = b.Id
			break
		}
	}

	// 拉取服务端整蛊配置
	prank, err := service.GetPrankConfig(a.profile.Site.Id, account.Platform)
	if err != nil {
		return fmt.Errorf("获取整蛊配置失败: %w", err)
	}

	// 写入 runtime
	glb.Runtime = &mytypes.RuntimeConfig{
		UserId:   a.profile.User.Id,
		SiteId:   a.profile.Site.Id,
		ArBoxId:  monsterBoxId,
		LiveUrl:  account.LiveUrl,
		Platform: account.Platform,
		Prank:    prank,
	}

	eventCb := func(event service.EventPayload) {
		if event.Type == service.EventStatus {
			runtime.EventsEmit(a.ctx, "event:status", event.Data)
			return
		}
		runtime.EventsEmit(a.ctx, "event:"+string(event.Type), event)
	}

	// 1) MQTT（两个平台都要）
	a.status = "connecting"
	runtime.EventsEmit(a.ctx, "event:status", "connecting")
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

	// 2) 根据平台构建客户端
	var client service.PrankClient
	switch account.Platform {
	case "kuaishou":
		a.status = "fetching_token"
		runtime.EventsEmit(a.ctx, "event:status", "fetching_token")
		info, ferr := initialize.FetchWssInfo(account.LiveUrl, 120*time.Second)
		if ferr != nil {
			a.status = "disconnected"
			runtime.EventsEmit(a.ctx, "event:status", "disconnected")
			return fmt.Errorf("获取 WSS token 失败: %w", ferr)
		}
		ksClient := service.NewKuaishouClient(prank, eventCb)
		wssURL := info.WssUrl
		if wssURL == "" {
			wssURL = "wss://livejs-ws-group5.gifshow.com/websocket"
		}
		if err := ksClient.Connect(wssURL, info.Token, info.LiveStreamId); err != nil {
			a.status = "disconnected"
			runtime.EventsEmit(a.ctx, "event:status", "disconnected")
			return err
		}
		client = ksClient

	case "douyin":
		a.status = "fetching_token"
		runtime.EventsEmit(a.ctx, "event:status", "fetching_token")
		// 5 分钟预算：覆盖用户在新 Chrome 里扫码登录抖音的时间
		info, ferr := initialize.FetchDouyinWssUrl(account.LiveUrl, 5*time.Minute)
		if ferr != nil {
			a.status = "disconnected"
			runtime.EventsEmit(a.ctx, "event:status", "disconnected")
			return fmt.Errorf("获取抖音 WSS URL 失败: %w", ferr)
		}
		dyClient, derr := service.NewDouyinPrankClient(info.WssUrl, info.Cookies, prank, eventCb)
		if derr != nil {
			a.status = "disconnected"
			runtime.EventsEmit(a.ctx, "event:status", "disconnected")
			return fmt.Errorf("抖音连接失败: %w", derr)
		}
		client = dyClient

	default:
		a.status = "disconnected"
		return fmt.Errorf("暂不支持的平台: %s", account.Platform)
	}

	a.client = client
	a.status = "connected"
	runtime.EventsEmit(a.ctx, "event:status", "connected")

	a.cfg.LastAccountId = liveAccountId
	a.persistConfig()

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
