package initialize

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type WssInfo struct {
	Token        string
	WssUrl       string
	LiveStreamId string
}

type websocketInfoResponse struct {
	Data struct {
		Result        int      `json:"result"`
		Token         string   `json:"token"`
		WebsocketUrls []string `json:"websocketUrls"`
	} `json:"data"`
}

func FetchWssInfo(liveUrl string, timeout time.Duration) (*WssInfo, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	// chrome-user-data 放在用户目录下，避免污染项目目录导致 Wails 文件监视器崩溃
	homeDir, _ := os.UserHomeDir()
	chromeDataDir := filepath.Join(homeDir, ".ks-prank", "chrome-user-data")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(chromeDataDir),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	var mu sync.Mutex
	type requestInfo struct {
		URL       string
		RequestID network.RequestID
	}
	var matched []requestInfo
	captchaNotified := false

	resultCh := make(chan *WssInfo, 1)
	errCh := make(chan error, 1)

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if strings.Contains(strings.ToLower(e.Request.URL), "websocketinfo") {
				mu.Lock()
				matched = append(matched, requestInfo{URL: e.Request.URL, RequestID: e.RequestID})
				mu.Unlock()
				log.Printf("[Chrome] 捕获请求: %s", e.Request.URL)
			}
		case *network.EventLoadingFinished:
			mu.Lock()
			var found *requestInfo
			for i := range matched {
				if matched[i].RequestID == e.RequestID {
					found = &matched[i]
					break
				}
			}
			mu.Unlock()
			if found == nil {
				return
			}

			go func(reqURL string, reqID network.RequestID) {
				var body []byte
				err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					body, err = network.GetResponseBody(reqID).Do(ctx)
					return err
				}))
				if err != nil {
					return
				}

				var resp websocketInfoResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					return
				}

				// result=400002 表示触发滑块验证
				if resp.Data.Result == 400002 {
					if !captchaNotified {
						captchaNotified = true
						log.Println("[Chrome] 检测到滑块验证，请在浏览器中完成验证")
						speak("检测到滑块验证，请在浏览器中滑动验证码")
					}
					return
				}

				if resp.Data.Result != 1 || resp.Data.Token == "" {
					errCh <- fmt.Errorf("websocketinfo 返回异常: result=%d", resp.Data.Result)
					return
				}

				// 从请求 URL 中提取 liveStreamId
				liveStreamId := parseLiveStreamId(reqURL)
				if liveStreamId == "" {
					errCh <- fmt.Errorf("无法从请求 URL 中提取 liveStreamId")
					return
				}

				wssUrl := ""
				if len(resp.Data.WebsocketUrls) > 0 {
					wssUrl = resp.Data.WebsocketUrls[0]
				}

				log.Printf("[Chrome] 成功获取 token=%s... liveStreamId=%s", resp.Data.Token[:20], liveStreamId)
				speak("验证通过，数据获取成功")

				resultCh <- &WssInfo{
					Token:        resp.Data.Token,
					WssUrl:       wssUrl,
					LiveStreamId: liveStreamId,
				}
			}(found.URL, found.RequestID)
		}
	})

	if err := chromedp.Run(ctx, network.Enable(), chromedp.Navigate(liveUrl)); err != nil {
		return nil, fmt.Errorf("Chrome 导航失败: %w", err)
	}

	log.Println("[Chrome] 已打开直播间页面，等待获取 websocketinfo...")

	select {
	case info := <-resultCh:
		return info, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("获取 websocketinfo 超时")
	}
}

// FetchDouyinWssUrl 在 Chrome 里打开抖音直播间，捕获 app_name=douyin_web 的 WSS URL。
// Douyin 页面加载后会自行建立弹幕 WebSocket，我们只做被动监听。
func FetchDouyinWssUrl(liveUrl string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	homeDir, _ := os.UserHomeDir()
	chromeDataDir := filepath.Join(homeDir, ".ks-prank", "chrome-user-data-dy")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(chromeDataDir),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	resultCh := make(chan string, 1)
	var once sync.Once

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*network.EventWebSocketCreated)
		if !ok {
			return
		}
		u := e.URL
		if !strings.Contains(u, "app_name=douyin_web") {
			return
		}
		preview := u
		if len(preview) > 160 {
			preview = preview[:160] + "..."
		}
		log.Printf("[Chrome] 捕获抖音 WSS: %s", preview)
		once.Do(func() { resultCh <- u })
	})

	if err := chromedp.Run(ctx, network.Enable(), chromedp.Navigate(liveUrl)); err != nil {
		return "", fmt.Errorf("Chrome 导航失败: %w", err)
	}
	log.Println("[Chrome] 已打开抖音直播间，等待 WebSocket 建立...")

	select {
	case u := <-resultCh:
		speak("抖音直播间数据获取成功")
		return u, nil
	case <-ctx.Done():
		return "", fmt.Errorf("获取抖音 WSS URL 超时")
	}
}

func parseLiveStreamId(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("liveStreamId")
}

func speak(text string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("say", "-v", "Ting-Ting", text).Start()
	case "windows":
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Speech; (New-Object System.Speech.Synthesis.SpeechSynthesizer).Speak('%s')`, text)
		exec.Command("powershell", "-Command", script).Start()
	}
}
