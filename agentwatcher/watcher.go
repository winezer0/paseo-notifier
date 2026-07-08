package agentwatcher

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
)

// Watcher 通过 MCP API 轮询监控 Agent 状态
type Watcher struct {
	daemonURL             string            // MCP 守护进程地址（如 http://127.0.0.1:6767/mcp/agents）
	interval              time.Duration     // 轮询间隔，默认 5s
	stuckDetectTimeout    time.Duration     // 卡死检测超时，UpdatedAt 超过此时长无变化则触发检查，0 禁用
	stuckRestartDelay     time.Duration     // 卡死确认后自动重启延迟，0 禁用自动重启
	maxRetries            int               // 自动重启最大重试次数
	continuePrompt        string            // 发送给卡死 Agent 的继续任务提示文本
	runningStatusInterval time.Duration     // 运行中状态心跳通知间隔，0 禁用
	notifier              Notifier          // 通知器接口，发送 Agent 事件通知
	sysNotifyFn           SystemNotifyFunc  // 系统事件通知回调（断连/重连）
	connState             ConnState         // 当前 MCP 守护进程连接状态，用于断连/重连检测
	prevAgents            map[string]*AgentState // Agent 状态快照（key=Agent ID），用于检测变化和卡死
	prevPermIDs           map[string]bool   // 已通知的权限请求 ID 集合，用于权限去重
	httpClient            *http.Client      // HTTP 客户端，默认超时 10s
	done                  chan struct{}     // 关闭信号，通知轮询循环退出
	reqID                 atomic.Int64     // MCP JSON-RPC 请求 ID 自增计数器（线程安全）
	ctx                   context.Context  // Watcher 生命周期上下文，用于取消 HTTP 请求
	cancel                context.CancelFunc // 取消函数
	managedAgents         map[string]bool   // 非空时只处理指定 Agent，用于测试隔离
	mu                    sync.Mutex        // 保护 prevAgents 并发读写（主循环 + 后台 goroutine）
}

// NewWatcher 根据配置创建 Agent 状态监控器
func NewWatcher(cfg config.MonitorConfig, notifier Notifier, continuePrompt string) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watcher{
		daemonURL:              cfg.DaemonURL,
		interval:               cfg.IntervalDuration(),
		stuckDetectTimeout:     cfg.StuckDetectDuration(),
		stuckRestartDelay:      cfg.StuckRestartDuration(),
		maxRetries:             cfg.StuckRestartRetry,
		continuePrompt:         continuePrompt,
		runningStatusInterval:  cfg.RunningStatusIntervalDuration(),
		notifier:               notifier,
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
	return int(w.reqID.Add(1))
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
	// 并行拉取 agents 和 permissions，互不阻塞
	var (
		agents    []AgentStatus
		agentsErr error
		perms     []PermissionRequest
		permsErr  error
	)
	done := make(chan struct{}, 2)
	go func() {
		agents, agentsErr = w.fetchAgents()
		done <- struct{}{}
	}()
	go func() {
		perms, permsErr = w.fetchPendingPermissions()
		done <- struct{}{}
	}()
	<-done
	<-done

	disconnected := agentsErr != nil
	if !disconnected && permsErr != nil {
		logging.Warnf("fetch permissions failed: %v", permsErr)
	}

	w.handleConnState(disconnected)
	if disconnected {
		return
	}

	for _, agent := range agents {
		w.detectAgentChange(agent)
	}
	w.detectStuckAgents(agents)
	w.checkRunningAgents(agents)
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
		w.mu.Lock()
		w.prevAgents = make(map[string]*AgentState)
		w.prevPermIDs = make(map[string]bool)
		w.mu.Unlock()
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