package agentwatcher

import (
	"context"
	"net/http"
	"time"

	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
)

// Watcher 通过 MCP API 轮询监控 Agent 状态
type Watcher struct {
	daemonURL          string
	interval           time.Duration
	stuckDetectTimeout time.Duration
	stuckRestartDelay  time.Duration
	maxRetries         int
	continuePrompt     string
	notifier           Notifier
	sysNotifyFn    SystemNotifyFunc
	connState      ConnState
	prevAgents     map[string]*AgentState
	prevPermIDs    map[string]bool
	httpClient     *http.Client
	done           chan struct{}
	reqID          int
	ctx            context.Context
	cancel         context.CancelFunc
	managedAgents  map[string]bool // 非空时只处理指定 Agent，用于测试隔离
}

// NewWatcher 根据配置创建 Agent 状态监控器
func NewWatcher(cfg config.MonitorConfig, notifier Notifier, continuePrompt string) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watcher{
		daemonURL:          cfg.DaemonURL,
		interval:           cfg.IntervalDuration(),
		stuckDetectTimeout: cfg.StuckDetectDuration(),
		stuckRestartDelay:  cfg.StuckRestartDuration(),
		maxRetries:         cfg.StuckRestartRetry,
		continuePrompt:     continuePrompt,
		notifier:           notifier,
		connState:    ConnConnected,
		prevAgents:   make(map[string]*AgentState),
		prevPermIDs:  make(map[string]bool),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 5,
				DisableCompression:  false,
			},
		},
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
		reqID:  0,
	}
}

// SetStuckDetectTimeout 设置卡死检测超时（秒），0 表示禁用
func (w *Watcher) SetStuckDetectTimeout(sec int) {
	if sec <= 0 {
		w.stuckDetectTimeout = 0
	} else {
		w.stuckDetectTimeout = time.Duration(sec) * time.Second
	}
}

// SetStuckRestartDelay 设置卡死重启延迟（秒），0 表示禁用自动重启
func (w *Watcher) SetStuckRestartDelay(sec int) {
	if sec <= 0 {
		w.stuckRestartDelay = 0
	} else {
		w.stuckRestartDelay = time.Duration(sec) * time.Second
	}
}

// SetMaxRetries 设置自动重启最大重试次数，默认 3
func (w *Watcher) SetMaxRetries(n int) {
	if n > 0 {
		w.maxRetries = n
	}
}

// SetContinuePrompt 设置继续任务的提示文本
func (w *Watcher) SetContinuePrompt(prompt string) {
	w.continuePrompt = prompt
}

// SetSystemNotifier 设置系统事件通知回调
func (w *Watcher) SetSystemNotifier(fn SystemNotifyFunc) {
	w.sysNotifyFn = fn
}

// SetManagedAgents 设置只处理指定 Agent ID 列表，空列表表示处理所有
// 用于测试隔离，避免影响其他 Agent
func (w *Watcher) SetManagedAgents(ids ...string) {
	w.managedAgents = make(map[string]bool, len(ids))
	for _, id := range ids {
		w.managedAgents[id] = true
	}
}

func (w *Watcher) nextID() int {
	w.reqID++
	return w.reqID
}

// Start 开始轮询监控
func (w *Watcher) Start() {
	logging.Infof("agent watcher started daemon=%s interval=%s", w.daemonURL, w.interval)

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		w.pollOnce()

		for {
			select {
			case <-ticker.C:
				w.pollOnce()
			case <-w.done:
				logging.Info("agent watcher stopped")
				return
			}
		}
	}()
}

// Stop 停止轮询监控
func (w *Watcher) Stop() {
	w.cancel()
	close(w.done)
}

func (w *Watcher) pollOnce() {
	// 一次探活决定全局连接状态
	agents, agentsErr := w.fetchAgents()

	disconnected := agentsErr != nil

	// 如果 agents 请求成功，再请求 permissions
	var perms []PermissionRequest
	if !disconnected {
		var permsErr error
		perms, permsErr = w.fetchPendingPermissions()
		if permsErr != nil {
			logging.Warnf("fetch permissions failed: %v", permsErr)
		}
	}

	w.handleConnState(disconnected)
	if disconnected {
		return
	}

	for _, agent := range agents {
		w.detectAgentChange(agent)
	}
	w.detectStuckAgents(agents)
	for _, perm := range perms {
		w.detectNewPermission(perm)
	}
}

// handleConnState 处理连接状态转换
func (w *Watcher) handleConnState(disconnected bool) {
	switch {
	case disconnected && w.connState == ConnConnected:
		w.connState = ConnDisconnected
		w.sendDisconnectedNotify()

	case !disconnected && w.connState == ConnDisconnected:
		w.connState = ConnConnected
		w.prevAgents = make(map[string]*AgentState)
		w.prevPermIDs = make(map[string]bool)
		w.sendReconnectedNotify()

	case disconnected && w.connState == ConnDisconnected:
		// 持续断开，不做操作

	case !disconnected && w.connState == ConnConnected:
		// 持续连接，不做操作
	}
}

func (w *Watcher) sendDisconnectedNotify() {
	logging.Warn("mcp daemon disconnected, agent notifications paused")
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(true, w.daemonURL)
	}
}

func (w *Watcher) sendReconnectedNotify() {
	logging.Info("mcp daemon reconnected, agent notifications resumed")
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(false, w.daemonURL)
	}
}