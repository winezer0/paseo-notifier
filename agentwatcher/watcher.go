package agentwatcher

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
)

// Watcher 通过 MCP API 轮询监控 Agent 状态，通过 WebSocket 追踪 subagent 完成
type Watcher struct {
	daemonURL             string                 // Paseo daemon 基础地址（如 http://127.0.0.1:6767）
	mcpURL                string                 // MCP 端点地址（daemonURL + "/mcp/agents"）
	interval              time.Duration
	stuckDetectTimeout    time.Duration
	stuckRestartDelay     time.Duration
	maxRetries            int
	continuePrompt           string
	stuckContinuePrompt      string
	subagentDoneContinuePrompt string              // 子任务全部完成后发送给主 agent 的继续提示
	autoContinue             bool
	notifyMinDuration        time.Duration         // 短于此时长完成的任务不通知
	runningStatusInterval    time.Duration
	subagentRunningInterval  time.Duration          // subagent 持续运行通知间隔
	notifier                Notifier
	sysNotifyFn           SystemNotifyFunc
	connState             ConnState
	prevAgents            map[string]*AgentState
	prevPermIDs           map[string]bool
	httpClient            *http.Client
	done                  chan struct{}
	reqID                 atomic.Int64
	ctx                   context.Context
	cancel                context.CancelFunc
	managedAgents         map[string]bool
	mu                    sync.Mutex
	wsClient              *DaemonWSClient          // WebSocket 客户端，接收 provider subagent 推送
	subagentTracker       *ProviderSubagentTracker  // provider subagent 状态追踪
	lastSubagentNotify    map[string]time.Time      // 上次发送 subagent 运行通知的时间（key=parentID）
	startedAt             time.Time
}

// NewWatcher 根据配置创建 Agent 状态监控器
func NewWatcher(cfg config.MonitorConfig, notifier Notifier, continuePrompt, stuckContinuePrompt string) *Watcher {
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
		daemonURL:             baseURL,
		mcpURL:                baseURL + "/mcp/agents",
		interval:              cfg.IntervalDuration(),
		stuckDetectTimeout:    cfg.StuckDetectDuration(),
		stuckRestartDelay:     cfg.StuckRestartDuration(),
		maxRetries:            cfg.StuckRestartRetry,
		continuePrompt:             continuePrompt,
		stuckContinuePrompt:       stuckContinuePrompt,
		subagentDoneContinuePrompt: subagentDoneDefaultPrompt(),
		autoContinue:             cfg.AutoContinue,
		notifyMinDuration:        cfg.NotifyMinDurationDuration(),
		runningStatusInterval:    cfg.RunningStatusIntervalDuration(),
		subagentRunningInterval:  cfg.SubagentRunningIntervalDuration(),
		notifier:                notifier,
		connState:               ConnConnected,
		prevAgents:              make(map[string]*AgentState),
		prevPermIDs:             make(map[string]bool),
		lastSubagentNotify:      make(map[string]time.Time),
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

// setupSubagentTracking 初始化 provider subagent 追踪器，注册 WS 消息处理器
func (w *Watcher) setupSubagentTracking() {
	if w.wsClient == nil {
		return
	}

	// 共享的 agent 查找函数，避免每处重复 fetchAgents
	lookupAgent := func(agentID string) AgentStatus {
		agents, err := w.fetchAgents()
		if err != nil {
			return AgentStatus{ID: agentID, ShortID: agentID}
		}
		for _, a := range agents {
			if a.ID == agentID {
				return a
			}
		}
		return AgentStatus{ID: agentID, ShortID: agentID}
	}

	w.subagentTracker = NewProviderSubagentTracker(
		// 全部完成回调
		func(parentAgentID string, subagents []ProviderSubagentStatus) {
			agent := lookupAgent(parentAgentID)
			ev := AgentEvent{
				Type:      EventAllSubagentsDone,
				Agent:     agent,
				Timestamp: time.Now(),
				Subagents: subagents,
			}
			if err := w.notifier.Notify(w.ctx, ev); err != nil {
				logging.Errorf("notify all subagents done failed agentId=%s err=%v", parentAgentID, err)
			} else {
				logging.Infof("all subagents done notified agentId=%s count=%d", agent.ShortID, len(subagents))
			}

			// 子任务完成后自动继续：父 agent 处于 idle/finished 时触发
			if w.autoContinue && w.continuePrompt != "" {
				if agent.Status == "idle" || (agent.AttentionReason != nil && *agent.AttentionReason == "finished") {
					logging.Infof("auto continue after subagents agentId=%s status=%s", agent.ShortID, agent.Status)
					prompt := w.subagentDoneContinuePrompt
					if prompt == "" {
						prompt = w.continuePrompt
					}
					if err := w.continueAgent(parentAgentID, prompt); err != nil {
						logging.Warnf("auto continue after subagents failed agentId=%s err=%v", agent.ShortID, err)
					} else {
						acEv := AgentEvent{
							Type:      EventAutoContinue,
							Agent:     agent,
							Timestamp: time.Now(),
						}
						if err := w.notifier.Notify(w.ctx, acEv); err != nil {
							logging.Errorf("notify auto continue after subagents failed agentId=%s err=%v", parentAgentID, err)
						}
					}
				}
			}
		},
		// 首个 subagent 出现回调
		func(parentAgentID string, subagent ProviderSubagentStatus) {
			agent := lookupAgent(parentAgentID)
			// 预占位运行通知间隔，避免 spawn 后立即触发 running 通知
			w.mu.Lock()
			w.lastSubagentNotify[parentAgentID] = time.Now()
			w.mu.Unlock()
			ev := AgentEvent{
				Type:      EventSubagentSpawned,
				Agent:     agent,
				Timestamp: time.Now(),
				Subagents: []ProviderSubagentStatus{subagent},
			}
			if err := w.notifier.Notify(w.ctx, ev); err != nil {
				logging.Errorf("notify subagent spawned failed agentId=%s err=%v", parentAgentID, err)
			} else {
				logging.Infof("subagent spawned notified agentId=%s sub=%s", agent.ShortID, subagent.SubagentID)
			}
		},
	)

	// 注册 WS 消息处理器
	w.wsClient.OnMessage(BuildUpdateType(), w.subagentTracker.HandleSubagentUpdate)
	w.wsClient.OnMessage(BuildListResponseType(), w.subagentTracker.HandleSubagentList)

	// 连接成功后请求所有活跃 agent 的 subagent 列表
	w.wsClient.OnConnected(func() {
		agents, err := w.fetchAgents()
		if err != nil {
			return
		}
		for _, a := range agents {
			if a.ArchivedAt != nil {
				continue
			}
			msg := BuildListRequest(fmt.Sprintf("list-%s-%d", a.ID, time.Now().UnixNano()), a.ID)
			if err := w.wsClient.Send(msg); err != nil {
				logging.Debugf("ws request subagent list failed agentId=%s err=%v", a.ShortID, err)
			}
		}
	})

	// 断开时重置追踪状态
	w.wsClient.OnDisconnected(func() {
		if w.subagentTracker != nil {
			w.subagentTracker.Reset()
		}
		w.lastSubagentNotify = make(map[string]time.Time)
	})
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
	case !disconnected && w.connState == ConnConnected:
	}
}

// checkRunningAgents 检查运行中的 Agent，定期发送心跳通知。
// 附带当前 subagent 进度汇总。
func (w *Watcher) checkRunningAgents(agents []AgentStatus) {
	if w.runningStatusInterval == 0 {
		return
	}
	now := time.Now()
	for _, agent := range agents {
		if w.shouldSkipRunningCheck(agent) {
			continue
		}
		prev, exists := w.prevAgents[agent.ID]
		if !exists {
			continue
		}

		lastUserTime := agent.LastUserMessageAt
		if lastUserTime == "" {
			lastUserTime = agent.CreatedAt
		}
		if lastUserTime == "" {
			continue
		}
		lastUserTimeParsed, err := time.Parse(time.RFC3339, lastUserTime)
		if err != nil {
			continue
		}
		sinceLastUser := now.Sub(lastUserTimeParsed)
		if sinceLastUser < w.runningStatusInterval {
			continue
		}

		w.mu.Lock()
		if prev.LastRunningNotify != nil && now.Sub(*prev.LastRunningNotify) < w.runningStatusInterval {
			w.mu.Unlock()
			continue
		}
		prev.LastRunningNotify = &now
		w.mu.Unlock()

		go func(agent AgentStatus) {
			entries := w.getAgentActivity(agent.ID)
			var subagents []ProviderSubagentStatus
			if w.subagentTracker != nil {
				subagents = w.subagentTracker.GetByParent(agent.ID)
			}
			ev := AgentEvent{
				Type:            EventRunningStatus,
				Agent:           agent,
				Timestamp:       now,
				ActivityEntries: entries,
				Subagents:       subagents,
			}
			if err := w.notifier.Notify(w.ctx, ev); err != nil {
				logging.Errorf("notify running status failed agentId=%s err=%v", agent.ID, err)
			} else {
				logging.Infof("running status notified agentId=%s title=%s idle=%s", agent.ShortID, agent.Title, sinceLastUser)
			}
		}(agent)
	}
}

// shouldSkipRunningCheck 判断 Agent 是否应跳过运行中状态检测
func (w *Watcher) shouldSkipRunningCheck(agent AgentStatus) bool {
	switch {
	case agent.ArchivedAt != nil:
		return true
	case agent.Status != "running":
		return true
	case agent.AttentionReason != nil && (*agent.AttentionReason == "finished" || *agent.AttentionReason == "error"):
		return true
	case len(w.managedAgents) > 0 && !w.managedAgents[agent.ID]:
		return true
	}
	return false
}

// shouldSkipStuckCheck 判断 Agent 是否应跳过卡死检测
func (w *Watcher) shouldSkipStuckCheck(agent AgentStatus) bool {
	switch {
	case agent.ArchivedAt != nil:
		return true
	case agent.Status != "running":
		return true
	case agent.AttentionReason != nil && (*agent.AttentionReason == "finished" || *agent.AttentionReason == "error"):
		return true
	case len(w.managedAgents) > 0 && !w.managedAgents[agent.ID]:
		return true
	}
	return false
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

// normalizeDaemonURL 兼容旧配置：若传入的 URL 以 /mcp/agents 结尾，自动剥离后缀返回基础地址
func normalizeDaemonURL(raw string) string {
	if strings.HasSuffix(raw, "/mcp/agents") {
		return raw[:len(raw)-len("/mcp/agents")]
	}
	return raw
}

// checkRunningSubagents 循环检测：当 subagent 持续运行时，每隔 subagentRunningInterval 发送通知
func (w *Watcher) checkRunningSubagents() {
	if w.subagentTracker == nil || w.subagentRunningInterval == 0 {
		return
	}
	now := time.Now()
	for _, parentID := range w.subagentTracker.GetTrackedParents() {
		if len(w.managedAgents) > 0 && !w.managedAgents[parentID] {
			continue
		}
		if !w.subagentTracker.HasRunningSubagents(parentID) {
			continue
		}

		w.mu.Lock()
		lastNotify, exists := w.lastSubagentNotify[parentID]
		if exists && now.Sub(lastNotify) < w.subagentRunningInterval {
			w.mu.Unlock()
			continue
		}
		w.lastSubagentNotify[parentID] = now
		w.mu.Unlock()

		go func(pid string) {
			agent := w.lookupAgent(pid)
			subagents := w.subagentTracker.GetByParent(pid)
			ev := AgentEvent{
				Type:      EventSubagentRunning,
				Agent:     agent,
				Timestamp: now,
				Subagents: subagents,
			}
			if err := w.notifier.Notify(w.ctx, ev); err != nil {
				logging.Errorf("notify subagent running failed agentId=%s err=%v", pid, err)
			} else {
				logging.Infof("subagent running notified agentId=%s running=%d", agent.ShortID, len(subagents))
			}
		}(parentID)
	}
}

// subagentDoneDefaultPrompt 子任务全部完成后发送给主 agent 的默认继续提示
func subagentDoneDefaultPrompt() string {
	return "检测到子任务可能都已经完成，请检查子任务状态，并继续完成主任务。"
}// lookupAgent 查找 agent 信息（fetchAgents 失败时返回仅有 ID 的占位 AgentStatus）
func (w *Watcher) lookupAgent(agentID string) AgentStatus {
	agents, err := w.fetchAgents()
	if err != nil {
		return AgentStatus{ID: agentID, ShortID: agentID}
	}
	for _, a := range agents {
		if a.ID == agentID {
			return a
		}
	}
	return AgentStatus{ID: agentID, ShortID: agentID}
}

// fetchAgentStatus 重新获取指定 Agent 的最新状态
// 返回 nil 表示 Agent 未找到或 fetch 失败
func (w *Watcher) fetchAgentStatus(agentID string) *AgentStatus {
	agents, err := w.fetchAgents()
	if err != nil {
		return nil
	}
	for i := range agents {
		if agents[i].ID == agentID {
			return &agents[i]
		}
	}
	return nil
}
