package agentwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/winezer0/paseo-notifier/logging"
)

// reconnectionBackoff 重连退避策略
var reconnectionBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// WSMessage WebSocket 消息信封
type WSMessage struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
}

// sessionMessage daemon 发出的 session 信封消息
type sessionMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// sessionInner 解包后的内层消息
type sessionInner struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}
type helloMessage struct {
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocolVersion"`
	ClientID        string          `json:"clientId"`
	ClientType      string          `json:"clientType"`
	AppVersion      string          `json:"appVersion,omitempty"`
	Capabilities    map[string]bool `json:"capabilities,omitempty"`
}

// DaemonWSClient 通过 WebSocket 连接 Paseo daemon，接收 provider subagent 推送
type DaemonWSClient struct {
	wsURL        string
	conn         *websocket.Conn
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	handlers     map[string]func(payload json.RawMessage) // 消息类型 → 处理器
	reconnectCh  chan struct{}
	onConnect    func() // 连接成功回调（用于重新拉取基线数据）
	onDisconnect func() // 断开回调
}

// NewDaemonWSClient 创建 WebSocket 客户端
// daemonURL 为 daemon 基础地址（如 http://127.0.0.1:6767），自动推导 WS 地址为 ws://host:port/ws
func NewDaemonWSClient(daemonURL string) (*DaemonWSClient, error) {
	u, err := url.Parse(daemonURL)
	if err != nil {
		return nil, fmt.Errorf("parse daemon URL: %w", err)
	}
	wsURL := fmt.Sprintf("ws://%s/ws", u.Host)

	ctx, cancel := context.WithCancel(context.Background())

	return &DaemonWSClient{
		wsURL:       wsURL,
		ctx:         ctx,
		cancel:      cancel,
		handlers:    make(map[string]func(json.RawMessage)),
		reconnectCh: make(chan struct{}, 1),
	}, nil
}

// OnMessage 注册消息类型处理器
func (c *DaemonWSClient) OnMessage(msgType string, handler func(payload json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[msgType] = handler
}

// OnConnected 注册连接成功回调
func (c *DaemonWSClient) OnConnected(fn func()) {
	c.onConnect = fn
}

// OnDisconnected 注册断开回调
func (c *DaemonWSClient) OnDisconnected(fn func()) {
	c.onDisconnect = fn
}

// Start 启动 WebSocket 连接，自动重连
func (c *DaemonWSClient) Start() {
	go c.connectLoop()
}

// Stop 停止 WebSocket 客户端
func (c *DaemonWSClient) Stop() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
	}
}

// Send 发送 WebSocket 消息
func (c *DaemonWSClient) Send(msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteJSON(msg)
}

// getHandlers 线程安全地复制 handler 列表
func (c *DaemonWSClient) getHandlers() map[string]func(json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]func(json.RawMessage), len(c.handlers))
	for k, v := range c.handlers {
		result[k] = v
	}
	return result
}

// connectLoop 连接循环，含指数退避重连
func (c *DaemonWSClient) connectLoop() {
	backoffIdx := 0
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connectAndRead(); err != nil {
			logging.Warnf("ws disconnected: %v", err)
		}

		if c.onDisconnect != nil {
			c.onDisconnect()
		}

		// 指数退避
		delay := reconnectionBackoff[backoffIdx]
		if backoffIdx < len(reconnectionBackoff)-1 {
			backoffIdx++
		}
		logging.Infof("ws reconnecting in %s (attempt %d)", delay, backoffIdx)

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// connectAndRead 建立连接、发送 Hello、读取消息循环
func (c *DaemonWSClient) connectAndRead() error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.DialContext(c.ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial %s: %w", c.wsURL, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

		// 发送 Hello 声明 protocol version 和 capability
		hello := helloMessage{
			Type:            "hello",
			ProtocolVersion: 1,
			ClientID:        "paseo-notifier",
			ClientType:      "cli",
			Capabilities:    map[string]bool{"provider_subagents": true},
		}
		if err := conn.WriteJSON(hello); err != nil {
			conn.Close()
			return fmt.Errorf("ws hello: %w", err)
		}

	logging.Infof("ws connected daemon=%s", c.wsURL)

	// 通知上层连接成功
	if c.onConnect != nil {
		c.onConnect()
	}

	// 读取循环
	for {
		select {
		case <-c.ctx.Done():
			conn.Close()
			return nil
		default:
		}

		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if closeErr, ok := err.(*websocket.CloseError); ok {
				logging.Debugf("ws closed code=%d text=%q", closeErr.Code, closeErr.Text)
			}
			conn.Close()
			return fmt.Errorf("ws read: %w", err)
		}

		// 先尝试按 session 信封解包
		var env sessionMessage
		if err := json.Unmarshal(rawMsg, &env); err != nil || env.Type != "session" {
			// 非 session 信封，按原始消息处理
			var msg WSMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				logging.Debugf("ws skip unparseable message: %s", string(rawMsg))
				continue
			}
			handlers := c.getHandlers()
			if h, ok := handlers[msg.Type]; ok {
				h(msg.Payload)
			}
			continue
		}

		// 解包内层消息
		var inner sessionInner
		if err := json.Unmarshal(env.Message, &inner); err != nil {
			logging.Debugf("ws skip unparseable inner message: %s", string(env.Message))
			continue
		}

		handlers := c.getHandlers()
		if h, ok := handlers[inner.Type]; ok {
			h(inner.Payload)
		}
	}
}
