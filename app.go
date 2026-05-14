package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
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
	ctx           context.Context
	mu            sync.Mutex
	client        service.PrankClient
	connectCancel context.CancelFunc
	connectToken  *struct{}
	cfg           *config.Config
	profile       *mytypes.Profile
	status        string // disconnected / connecting / connected / fetching_token
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
	cancel := a.connectCancel
	client := a.client
	a.connectCancel = nil
	a.connectToken = nil
	a.client = nil
	a.status = "disconnected"
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if client != nil {
		client.Close()
	}
	initialize.CloseMqtt()
	glb.Runtime = nil
}

func (a *App) stopConnection() bool {
	a.mu.Lock()
	cancel := a.connectCancel
	client := a.client
	if cancel == nil && client == nil {
		a.mu.Unlock()
		return false
	}
	a.connectCancel = nil
	a.connectToken = nil
	a.client = nil
	a.status = "disconnected"
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if client != nil {
		client.Close()
	}
	initialize.CloseMqtt()
	glb.Runtime = nil
	runtime.EventsEmit(a.ctx, "event:status", "disconnected")
	return true
}

func (a *App) finishConnect(token *struct{}, client service.PrankClient) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connectToken != token {
		return false
	}
	a.connectCancel = nil
	a.connectToken = nil
	a.client = client
	a.status = "connected"
	return true
}

func (a *App) failConnect(token *struct{}) {
	matched := false
	var cancel context.CancelFunc
	a.mu.Lock()
	if a.connectToken == token {
		cancel = a.connectCancel
		a.connectCancel = nil
		a.connectToken = nil
		a.status = "disconnected"
		matched = true
	}
	a.mu.Unlock()
	if !matched {
		return
	}
	if cancel != nil {
		cancel()
	}
	initialize.CloseMqtt()
	glb.Runtime = nil
	runtime.EventsEmit(a.ctx, "event:status", "disconnected")
}

func (a *App) clearFinishedClient(client service.PrankClient) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != client {
		return false
	}
	a.client = nil
	a.status = "disconnected"
	return true
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
	a.stopConnection()

	a.cfg.AuthToken = ""
	a.cfg.LastAccountId = ""
	a.profile = nil
	service.SetAuthToken("")
	a.persistConfig()
	return nil
}

// GetProfile 主动刷新账号资料（登录后调用）
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

// GetPrankRules 拉取指定直播账号对应（site_id + platform）的礼物/弹幕/点赞规则。
// 仅依赖已加载的 profile，无需先连接直播间。
func (a *App) GetPrankRules(liveAccountId string) (*mytypes.PrankConfigData, error) {
	if a.profile == nil {
		return nil, fmt.Errorf("请先加载账号资料")
	}
	if a.profile.Site == nil {
		return nil, fmt.Errorf("当前账号未绑定场地")
	}
	var account *mytypes.LiveAccount
	for _, acc := range a.profile.LiveAccounts {
		if acc.Id == liveAccountId {
			account = acc
			break
		}
	}
	if account == nil {
		return nil, fmt.Errorf("未找到指定的直播账号")
	}
	return service.GetPrankConfig(a.profile.Site.Id, account.Platform)
}

// ===== 连接 =====

// GetStatus 返回当前连接状态
func (a *App) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// Connect 用指定的直播账号连接。需要先 Login + GetProfile 过。
func (a *App) Connect(liveAccountId string) error {
	a.mu.Lock()
	if a.client != nil || a.connectCancel != nil {
		a.mu.Unlock()
		return fmt.Errorf("已经连接中，请先断开")
	}
	if a.profile == nil {
		a.mu.Unlock()
		return fmt.Errorf("请先加载账号资料")
	}
	if a.profile.Site == nil {
		a.mu.Unlock()
		return fmt.Errorf("当前账号未绑定场地")
	}

	var account mytypes.LiveAccount
	foundAccount := false
	for _, acc := range a.profile.LiveAccounts {
		if acc.Id == liveAccountId {
			account = *acc
			foundAccount = true
			break
		}
	}
	if !foundAccount {
		a.mu.Unlock()
		return fmt.Errorf("未找到指定的直播账号")
	}
	if !account.Enabled {
		a.mu.Unlock()
		return fmt.Errorf("该直播账号已停用")
	}
	if account.LiveUrl == "" {
		a.mu.Unlock()
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

	userId := a.profile.User.Id
	siteId := a.profile.Site.Id
	prankDeviceSn := a.profile.PrankDeviceSn
	connCtx, cancel := context.WithCancel(context.Background())
	token := &struct{}{}
	a.connectCancel = cancel
	a.connectToken = token
	a.status = "connecting"
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "event:status", "connecting")

	checkCanceled := func() error {
		if connCtx.Err() != nil {
			return fmt.Errorf("连接已取消")
		}
		return nil
	}

	// 拉取服务端整蛊配置
	prank, err := service.GetPrankConfig(siteId, account.Platform)
	if err != nil {
		a.failConnect(token)
		return fmt.Errorf("获取整蛊配置失败: %w", err)
	}
	if err := checkCanceled(); err != nil {
		a.failConnect(token)
		return err
	}

	// 写入 runtime
	glb.Runtime = &mytypes.RuntimeConfig{
		UserId:        userId,
		SiteId:        siteId,
		ArBoxId:       monsterBoxId,
		LiveUrl:       account.LiveUrl,
		Platform:      account.Platform,
		PrankDeviceSn: prankDeviceSn,
		Prank:         prank,
	}

	eventCb := func(event service.EventPayload) {
		if event.Type == service.EventStatus {
			runtime.EventsEmit(a.ctx, "event:status", event.Data)
			return
		}
		runtime.EventsEmit(a.ctx, "event:"+string(event.Type), event)
	}

	mqttCfg, err := initialize.FetchMqttConfig()
	if err != nil {
		a.failConnect(token)
		return fmt.Errorf("获取 MQTT 配置失败: %w", err)
	}
	if err := checkCanceled(); err != nil {
		a.failConnect(token)
		return err
	}
	if err := initialize.InitMqtt(mqttCfg); err != nil {
		a.failConnect(token)
		return fmt.Errorf("MQTT 连接失败: %w", err)
	}
	if err := checkCanceled(); err != nil {
		a.failConnect(token)
		return err
	}

	// 2) 根据平台构建客户端
	var client service.PrankClient
	switch account.Platform {
	case "kuaishou":
		a.mu.Lock()
		if a.connectToken == token {
			a.status = "fetching_token"
		}
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "event:status", "fetching_token")
		info, ferr := initialize.FetchWssInfoContext(connCtx, account.LiveUrl, 120*time.Second)
		if ferr != nil {
			a.failConnect(token)
			return fmt.Errorf("获取 WSS token 失败: %w", ferr)
		}
		if err := checkCanceled(); err != nil {
			a.failConnect(token)
			return err
		}
		ksClient := service.NewKuaishouClient(prank, eventCb)
		wssURL := info.WssUrl
		if wssURL == "" {
			wssURL = "wss://livejs-ws-group5.gifshow.com/websocket"
		}
		if err := ksClient.Connect(wssURL, info.Token, info.LiveStreamId); err != nil {
			a.failConnect(token)
			return err
		}
		client = ksClient

	case "douyin":
		a.mu.Lock()
		if a.connectToken == token {
			a.status = "fetching_token"
		}
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "event:status", "fetching_token")
		// 5 分钟预算：覆盖用户在新 Chrome 里扫码登录抖音的时间
		info, ferr := initialize.FetchDouyinWssUrlContext(connCtx, account.LiveUrl, 5*time.Minute)
		if ferr != nil {
			a.failConnect(token)
			return fmt.Errorf("获取抖音 WSS URL 失败: %w", ferr)
		}
		if err := checkCanceled(); err != nil {
			a.failConnect(token)
			return err
		}
		dyClient, derr := service.NewDouyinPrankClient(info.WssUrl, info.Cookies, prank, eventCb)
		if derr != nil {
			a.failConnect(token)
			return fmt.Errorf("抖音连接失败: %w", derr)
		}
		client = dyClient

	default:
		a.failConnect(token)
		return fmt.Errorf("暂不支持的平台: %s", account.Platform)
	}

	if !a.finishConnect(token, client) {
		cancel()
		client.Close()
		return fmt.Errorf("连接已取消")
	}
	cancel()
	runtime.EventsEmit(a.ctx, "event:status", "connected")

	a.cfg.LastAccountId = liveAccountId
	a.persistConfig()

	go func() {
		client.Listen()
		client.Close()
		if a.clearFinishedClient(client) {
			initialize.CloseMqtt()
			glb.Runtime = nil
			runtime.EventsEmit(a.ctx, "event:status", "disconnected")
		}
	}()

	return nil
}

func (a *App) Disconnect() error {
	if !a.stopConnection() {
		return fmt.Errorf("未连接")
	}
	return nil
}

// PlayCarStream 用本机 ffplay 拉取整蛊车的 RTSP 视频流(同局域网直连)。
// 要求 ffplay 在 PATH 中。
func (a *App) PlayCarStream(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("无效的 IP: %q", ip)
	}
	rtspURL := fmt.Sprintf("rtsp://%s/live/0", ip)
	cmd := exec.Command(
		"ffplay",
		"-rtsp_transport", "tcp",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		rtspURL,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ffplay 失败(请确认已安装并加入 PATH): %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	log.Printf("[PlayCarStream] 已启动 ffplay pid=%d url=%s", cmd.Process.Pid, rtspURL)
	return nil
}
