package douyincrawler

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type DouyinClient struct {
	conn    *websocket.Conn
	connMu  sync.RWMutex
	writeMu sync.Mutex

	handler  MessageHandler
	stopChan chan os.Signal
	quit     chan struct{}
	closeOnce sync.Once

	dialer  websocket.Dialer
	headers http.Header
	wssURL  string

	consecutiveReconnects atomic.Uint64
	lastConnectedMu       sync.RWMutex
	lastConnectedAt       time.Time
	reconnectDelayPlan    []time.Duration

	heartbeatMu       sync.RWMutex
	heartbeatInterval time.Duration
}

func NewDouyinClient(wssURL string, config MessageHandlerConfig) (*DouyinClient, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	headers := http.Header{}
	headers.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36")
	headers.Add("Cookie", `ttwid=1%7C7ZLJzwjjEw7NLeADTpVd-3eId-ZEIg0jpCEzTV9p_2A%7C1677681848%7C4ff4f97328ddc18b6d46c259bc26a05d2e654b50e3f21b27b8f9e9e8f9fcec82`)

	conn, _, err := dialer.Dial(wssURL, headers)
	if err != nil {
		return nil, err
	}

	c := &DouyinClient{
		conn:              conn,
		handler:           NewMessageHandler(config),
		stopChan:          make(chan os.Signal, 1),
		quit:              make(chan struct{}),
		dialer:            dialer,
		headers:           headers,
		wssURL:            wssURL,
		reconnectDelayPlan: []time.Duration{
			0,
			0,
			0,
			1 * time.Second,
			3 * time.Second,
			5 * time.Second,
			10 * time.Second,
		},
		heartbeatInterval: 30 * time.Second,
	}
	c.markConnected("初次连接成功")
	c.setupConn(conn)
	return c, nil
}

func (c *DouyinClient) Start() error {
	signal.Notify(c.stopChan, os.Interrupt)
	defer signal.Stop(c.stopChan)

	for {
		conn := c.getConn()
		if conn == nil {
			if err := c.reconnect(); err != nil {
				if c.isClosed() {
					return nil
				}
				return err
			}
			conn = c.getConn()
		}

		connErrCh := make(chan error, 2)
		go c.receiveMessages(conn, connErrCh)
		go c.heartbeat(conn, connErrCh)

		select {
		case <-c.stopChan:
			return c.Shutdown()
		case <-c.quit:
			return nil
		case err := <-connErrCh:
			if c.isClosed() {
				return nil
			}
			log.Printf(
				"抖音连接异常，准备立刻重连: %v (连续重连次数=%d, 上次成功连接时间=%s)",
				err,
				c.getConsecutiveReconnects(),
				c.getLastConnectedAt().Format(time.RFC3339),
			)
			if recErr := c.reconnect(); recErr != nil {
				if c.isClosed() {
					return nil
				}
				return recErr
			}
		}
	}
}

func (c *DouyinClient) receiveMessages(conn *websocket.Conn, connErrCh chan<- error) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if c.getConn() != conn || c.isClosed() {
				return
			}
			c.notifyConnError(connErrCh, err)
			return
		}

		if err := c.handler.ProcessMessage(message, c); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	}
}

func (c *DouyinClient) heartbeat(conn *websocket.Conn, connErrCh chan<- error) {
	currentInterval := c.getHeartbeatInterval()
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.quit:
			return
		case <-ticker.C:
			if c.getConn() != conn {
				return
			}
			if err := c.writeToConn(conn, websocket.PingMessage, nil); err != nil {
				c.notifyConnError(connErrCh, fmt.Errorf("发送心跳失败: %w", err))
				return
			}
			newInterval := c.getHeartbeatInterval()
			if newInterval != currentInterval {
				currentInterval = newInterval
				ticker.Reset(currentInterval)
				log.Printf("心跳间隔更新为: %s", currentInterval)
			}
		}
	}
}

func (c *DouyinClient) Shutdown() error {
	c.closeOnce.Do(func() {
		close(c.quit)
	})

	conn := c.getConn()
	if conn != nil {
		c.writeMu.Lock()
		err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(3*time.Second),
		)
		c.writeMu.Unlock()
		if err != nil {
			log.Println("关闭消息发送错误:", err)
		}
		c.closeConn(conn)
	}
	return nil
}

func (c *DouyinClient) SendMessage(msgType int, data []byte) error {
	conn := c.getConn()
	if conn == nil {
		return fmt.Errorf("websocket 未连接")
	}
	return c.writeToConn(conn, msgType, data)
}

func (c *DouyinClient) UpdateHeartbeatDuration(raw uint64) {
	if raw == 0 {
		return
	}

	interval := parseHeartbeatDuration(raw)
	c.heartbeatMu.Lock()
	if c.heartbeatInterval != interval {
		c.heartbeatInterval = interval
		log.Printf("服务端下发心跳间隔: raw=%d, parsed=%s", raw, interval)
	}
	c.heartbeatMu.Unlock()
}

func (c *DouyinClient) getHeartbeatInterval() time.Duration {
	c.heartbeatMu.RLock()
	defer c.heartbeatMu.RUnlock()
	return c.heartbeatInterval
}

func parseHeartbeatDuration(raw uint64) time.Duration {
	// 经验值：小于1000一般是秒；否则按毫秒处理。
	if raw < 1000 {
		d := time.Duration(raw) * time.Second
		if d < 5*time.Second {
			return 5 * time.Second
		}
		if d > 2*time.Minute {
			return 2 * time.Minute
		}
		return d
	}
	d := time.Duration(raw) * time.Millisecond
	if d < 5*time.Second {
		return 5 * time.Second
	}
	if d > 2*time.Minute {
		return 2 * time.Minute
	}
	return d
}

func (c *DouyinClient) reconnect() error {
	c.disconnectCurrentConn()

	for {
		if c.isClosed() {
			return nil
		}

		conn, _, err := c.dialer.Dial(c.wssURL, c.headers)
		if err == nil {
			c.setupConn(conn)
			c.setConn(conn)
			c.markConnected("抖音连接重连成功")
			return nil
		}

		current := c.consecutiveReconnects.Add(1)
		wait := c.getReconnectDelay(current)
		log.Printf(
			"抖音重连失败: %v (连续重连次数=%d, 上次成功连接时间=%s)，%s 后重试",
			err,
			current,
			c.getLastConnectedAt().Format(time.RFC3339),
			wait,
		)
		select {
		case <-c.quit:
			return nil
		case <-time.After(wait):
		}
	}
}

func (c *DouyinClient) setupConn(conn *websocket.Conn) {
	const pongWait = 90 * time.Second

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(appData string) error {
		log.Printf("收到 PONG: %s", appData)
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetPingHandler(func(appData string) error {
		log.Printf("收到 PING: %s", appData)
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})
}

func (c *DouyinClient) writeToConn(conn *websocket.Conn, msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteMessage(msgType, data)
}

func (c *DouyinClient) notifyConnError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func (c *DouyinClient) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *DouyinClient) setConn(conn *websocket.Conn) {
	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()
}

func (c *DouyinClient) disconnectCurrentConn() {
	conn := c.getConn()
	if conn == nil {
		return
	}
	c.closeConn(conn)
	c.setConn(nil)
}

func (c *DouyinClient) closeConn(conn *websocket.Conn) {
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *DouyinClient) isClosed() bool {
	select {
	case <-c.quit:
		return true
	default:
		return false
	}
}

func (c *DouyinClient) markConnected(reason string) {
	now := time.Now()
	c.lastConnectedMu.Lock()
	c.lastConnectedAt = now
	c.lastConnectedMu.Unlock()
	c.consecutiveReconnects.Store(0)
	log.Printf("%s (上次成功连接时间=%s)", reason, now.Format(time.RFC3339))
}

func (c *DouyinClient) getConsecutiveReconnects() uint64 {
	return c.consecutiveReconnects.Load()
}

func (c *DouyinClient) getLastConnectedAt() time.Time {
	c.lastConnectedMu.RLock()
	defer c.lastConnectedMu.RUnlock()
	if c.lastConnectedAt.IsZero() {
		return time.Now()
	}
	return c.lastConnectedAt
}

func (c *DouyinClient) getReconnectDelay(attempt uint64) time.Duration {
	if len(c.reconnectDelayPlan) == 0 {
		return 0
	}
	if attempt == 0 {
		return c.reconnectDelayPlan[0]
	}
	idx := int(attempt - 1)
	if idx >= len(c.reconnectDelayPlan) {
		return c.reconnectDelayPlan[len(c.reconnectDelayPlan)-1]
	}
	return c.reconnectDelayPlan[idx]
}
