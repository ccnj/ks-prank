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
	return FetchWssInfoContext(context.Background(), liveUrl, timeout)
}

func FetchWssInfoContext(parent context.Context, liveUrl string, timeout time.Duration) (*WssInfo, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if parent == nil {
		parent = context.Background()
	}

	// chrome-user-data 放在用户目录下，避免污染项目目录导致 Wails 文件监视器崩溃
	homeDir, _ := os.UserHomeDir()
	chromeDataDir := filepath.Join(homeDir, ".ks-prank", "chrome-user-data")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(chromeDataDir),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, opts...)
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
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("获取 websocketinfo 已取消")
		}
		return nil, fmt.Errorf("获取 websocketinfo 超时")
	}
}

// DouyinWssInfo 抖音 WSS 拨号所需信息：URL 本身 + 浏览器当前 .douyin.com 的 Cookie
// Cookie 以 `name1=val1; name2=val2` 的形式返回，可直接塞进 Cookie header
type DouyinWssInfo struct {
	WssUrl  string
	Cookies string
}

// FetchDouyinWssUrl 在 Chrome 里打开抖音直播间，捕获 app_name=douyin_web 的 WSS URL，
// 并抓取当前浏览器 .douyin.com 域下的 Cookie，供 Go 拨号器直接透传。
// Douyin 页面加载后会自行建立弹幕 WebSocket，我们只做被动监听。
func FetchDouyinWssUrl(liveUrl string, timeout time.Duration) (*DouyinWssInfo, error) {
	return FetchDouyinWssUrlContext(context.Background(), liveUrl, timeout)
}

func FetchDouyinWssUrlContext(parent context.Context, liveUrl string, timeout time.Duration) (*DouyinWssInfo, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if parent == nil {
		parent = context.Background()
	}

	homeDir, _ := os.UserHomeDir()
	chromeDataDir := filepath.Join(homeDir, ".ks-prank", "chrome-user-data-dy")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(chromeDataDir),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, opts...)
	defer allocCancel()

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	var mu sync.Mutex
	var candidates []string
	resultCh := make(chan string, 1)
	settleTimer := time.AfterFunc(3*time.Second, func() {
		mu.Lock()
		defer mu.Unlock()
		best := pickBestDouyinWss(candidates)
		if best != "" {
			select {
			case resultCh <- best:
			default:
			}
		}
	})
	settleTimer.Stop() // 初始不启动，等第一个 URL 到达才 Reset

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*network.EventWebSocketCreated)
		if !ok {
			return
		}
		u := e.URL
		// 只认直播间弹幕/礼物主通道；抖音首页在登录时会起 bytelink 等无关 WS，
		// 要挡掉，否则它们会提前触发 settleTimer 并被 fallback 选中，导致拨错通道。
		if !strings.Contains(u, "app_name=douyin_web") {
			return
		}
		if !strings.Contains(u, "/webcast/im/push/") {
			return
		}
		mu.Lock()
		candidates = append(candidates, u)
		idx := len(candidates)
		mu.Unlock()
		preview := u
		if len(preview) > 160 {
			preview = preview[:160] + "..."
		}
		log.Printf("[Chrome] 捕获候选 WSS #%d: %s", idx, preview)
		// 3 秒内收集所有候选，然后选最合适的
		settleTimer.Reset(3 * time.Second)
	})

	// 必须「先登录、后建连」——页面 JS 建 WebSocket 时会把当时的 Cookie 一起交给服务端，
	// 建连之后再登录就晚了，服务端只会按匿名身份推弹幕、不推礼物。
	// 所以先把 Chrome 导航到 www.douyin.com 首页，等 sessionid Cookie 出现再跳直播间。
	if err := chromedp.Run(ctx, network.Enable(), chromedp.Navigate("https://www.douyin.com/")); err != nil {
		return nil, fmt.Errorf("Chrome 导航首页失败: %w", err)
	}
	if err := waitForDouyinLogin(ctx); err != nil {
		return nil, err
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(liveUrl)); err != nil {
		return nil, fmt.Errorf("Chrome 导航直播间失败: %w", err)
	}
	log.Println("[Chrome] 已打开抖音直播间，等待 WebSocket 建立...")

	select {
	case u := <-resultCh:
		log.Printf("[Chrome] 最终选中 WSS（共 %d 个候选）: %s", func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(candidates)
		}(), u)

		cookies, cerr := extractDouyinCookies(ctx)
		if cerr != nil {
			log.Printf("[Chrome] 抓取 Cookie 失败: %v（继续用空 Cookie 拨号，礼物可能拿不到）", cerr)
		} else {
			log.Printf("[Chrome] 抓到 %d 个 .douyin.com Cookie", strings.Count(cookies, ";")+1)
		}
		speak("抖音直播间数据获取成功")
		return &DouyinWssInfo{WssUrl: u, Cookies: cookies}, nil
	case <-ctx.Done():
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("获取抖音 WSS URL 已取消")
		}
		return nil, fmt.Errorf("获取抖音 WSS URL 超时")
	}
}

// waitForDouyinLogin 轮询 Cookie，直到出现 sessionid（抖音登录凭证）再返回。
// 未登录时播报语音提示一次，后续每 30 秒复播一次；Chrome 里用户完成登录后会自动继续。
func waitForDouyinLogin(ctx context.Context) error {
	const loginCookieName = "sessionid"
	notifiedAt := time.Time{}
	for {
		cookies, err := readCookies(ctx)
		if err != nil {
			return fmt.Errorf("读取 Cookie 失败: %w", err)
		}
		for _, c := range cookies {
			if !strings.Contains(c.Domain, "douyin.com") {
				continue
			}
			if c.Name == loginCookieName && c.Value != "" {
				log.Printf("[Chrome] 检测到抖音登录态（sessionid 长度=%d），等待后续 Cookie 落位", len(c.Value))
				// sessionid 只是第一批下发的 Cookie，后续还有 sessionid_ss / sid_guard / ttwid 轮换等。
				// 如果立即跳直播间，页面可能用半登录态建 WS，导致 URL 签名和我们抓到的 Cookie 不匹配。
				time.Sleep(3 * time.Second)
				return nil
			}
		}

		if notifiedAt.IsZero() || time.Since(notifiedAt) > 30*time.Second {
			log.Println("[Chrome] 检测到当前未登录抖音账号，请在浏览器中登录")
			speak("检测到当前未登录抖音账号，请先登录")
			notifiedAt = time.Now()
		}

		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return fmt.Errorf("等待抖音登录已取消")
			}
			return fmt.Errorf("等待抖音登录超时")
		case <-time.After(2 * time.Second):
		}
	}
}

// readCookies 是 network.GetCookies 的包装，返回当前浏览器会话里的所有 Cookie。
func readCookies(ctx context.Context) ([]*network.Cookie, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		got, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		cookies = got
		return nil
	}))
	return cookies, err
}

// extractDouyinCookies 从当前 chromedp 上下文里取出所有域名含 douyin.com 的 Cookie，
// 拼成可直接塞进 HTTP Cookie 头的字符串。
func extractDouyinCookies(ctx context.Context) (string, error) {
	cookies, err := readCookies(ctx)
	if err != nil {
		return "", err
	}

	var pairs []string
	for _, c := range cookies {
		if !strings.Contains(c.Domain, "douyin.com") {
			continue
		}
		pairs = append(pairs, c.Name+"="+c.Value)
	}
	return strings.Join(pairs, "; "), nil
}

// pickBestDouyinWss 在多个候选里挑最可能是 IM push 主通道的那个：
// 优先选 path 含 /webcast/im/push/v2/ 且 identity=audience 的 URL。
// 多个都匹配时选最后一个（最新建立的，cursor 通常最准）。
func pickBestDouyinWss(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	var best string
	for _, u := range candidates {
		if strings.Contains(u, "/webcast/im/push/v2/") && strings.Contains(u, "identity=audience") {
			best = u
		}
	}
	if best != "" {
		return best
	}
	return candidates[len(candidates)-1]
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
