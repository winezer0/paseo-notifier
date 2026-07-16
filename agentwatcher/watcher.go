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

// Watcher 通过 MCP API 轮询监控 Agent 状态，通过 WebSocket 追踪 subagent 完成
type Watcher struct {
	daemonURL                  string // Paseo daemon 基础地址（如 http://127.0.0.1:6767）
	mcpURL                     string // MCP 端点地址（daemonURL + "/mcp/agents"）
	interval                   time.Duration
	stuckDetectTimeout         time.Duration
	stuckRestartDelay          time.Duration
	maxRetries                 int
	continuePrompt             string
	stuckContinuePrompt        string
	subagentDoneContinuePrompt string // 子任务全部完成后发送给主 agent 的继续提示
	autoContinueKeyword        bool   // 匹配关键字自动继续
	autoContinueSubagent       bool   // 子任务完成后自动继续
	notifyMinDuration          time.Duration // 短于此时长完成的任务不通知
	runningStatusInterval      time.Duration
	subagentRunningInterval    time.Duration // subagent 持续运行通知间隔
	notifier                   Notifier
	sysNotifyFn                SystemNotifyFunc
	connState                  ConnState
	prevAgents                 map[string]*AgentState
	prevPermIDs                map[string]bool
	httpClient                 *http.Client
	done                       chan struct{}
	reqID                      atomic.Int64
	ctx                        context.Context
	cancel                     context.CancelFunc
	managedAgents              map[string]bool
	mu                         sync.Mutex
	wsClient                   *DaemonWSClient          // WebSocket 客户端，接收 provider subagent 推送
	subagentTracker            *ProviderSubagentTracker // provider subagent 状态追踪
	lastSubagentNotify         map[string]time.Time     // 上次发送 subagent 运行通知的时间（key=parentID）
	events                     map[string]bool          // 事件开关映射，空表示全部启用
	startedAt                  time.Time
}

// NewWatcher 根据配置创建 Agent 状态监控器
func NewWatcher(cfg config.MonitorConfig, notifier Notifier, continuePrompt, stuckContinuePrompt, subagentDonePrompt string) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())

	// 兼容旧配置：自动剥离 /mcp/agents 后缀，统一为基础地址
	baseURL := normalizeDaemonURL(cfg.DaemonURL)
	if baseURL != cfg.DaemonURL {
		logging.Infof("daemon URL normalized: %s → %s", cfg.DaemonURL, baseURL)
	}

	wsClient, err := NewDaemonWSClient(baseURL)
	if err != nil {
		logging.Warnf("failed to create ws client: %v, subagent tracking disabled", err)
	}

	return &Watcher{
		daemonURL:                  baseURL,
		mcpURL:                     baseURL + "/mcp/agents",
		interval:                   cfg.IntervalDuration(),
		stuckDetectTimeout:         cfg.StuckDetectDuration(),
		stuckRestartDelay:          cfg.StuckRestartDuration(),
		maxRetries:                 cfg.StuckRestartRetry,
		continuePrompt:             continuePrompt,
		stuckContinuePrompt:        stuckContinuePrompt,
		subagentDoneContinuePrompt: subagentDonePrompt,
		autoContinueKeyword:        cfg.AutoContinueKeyword,
		autoContinueSubagent:       cfg.AutoContinueSubagent,
		notifyMinDuration:          cfg.NotifyMinDurationDuration(),
		runningStatusInterval:      cfg.RunningStatusIntervalDuration(),
		subagentRunningInterval:    cfg.SubagentRunningIntervalDuration(),
		notifier:                   notifier,
		connState:                  ConnConnected,
		prevAgents:                 make(map[string]*AgentState),
		prevPermIDs:                make(map[string]bool),
		lastSubagentNotify:         make(map[string]time.Time),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 5,
				DisableCompression:  false,
			},
		},
		done:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		wsClient:  wsClient,
		startedAt: time.Now(),
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

// SetMaxRetries 设置自动重启最大重试次数
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

// SetEvents 设置事件开关映射
func (w *Watcher) SetEvents(events map[string]bool) {
	w.events = events
}

// SetManagedAgents 设置只处理指定 Agent ID 列表
func (w *Watcher) SetManagedAgents(ids ...string) {
	w.managedAgents = make(map[string]bool, len(ids))
	for _, id := range ids {
		w.managedAgents[id] = true
	}
}

func (w *Watcher) nextID() int {
	return int(w.reqID.Add(1))
}

// Start 开始轮询监控和 WebSocket 连接
func (w *Watcher) Start() {
	logging.Infof("agent watcher started daemon=%s interval=%s", w.daemonURL, w.interval)

	// 启动 provider subagent 追踪
	w.setupSubagentTracking()

	// 启动 WebSocket 客户端
	if w.wsClient != nil {
		w.wsClient.Start()
	}

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

// Stop 停止轮询监控和 WebSocket 连接
func (w *Watcher) Stop() {
	w.cancel()
	close(w.done)
	if w.wsClient != nil {
		w.wsClient.Stop()
	}
}

func (w *Watcher) pollOnce() {
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
	w.checkRunningSubagents()
	for _, perm := range perms {
		w.detectNewPermission(perm)
	}
}
