package agentwatcher

import (
	"fmt"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// isEventEnabled 检查事件是否启用（未配置默认启用）
func (w *Watcher) isEventEnabled(eventType EventType) bool {
	on, exists := w.events[string(eventType)]
	if !exists {
		return true
	}
	return on
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

			// 子任务完成后自动继续：父 agent 空闲时立即发送，running 时延迟重试（10s 窗口）
			if w.autoContinueSubagent && w.subagentDoneContinuePrompt != "" {
				w.triggerAutoContinueAfterSubagents(parentAgentID, agent)
			}
		},
		// 首个 subagent 出现回调
		func(parentAgentID string, subagent ProviderSubagentStatus) {
			agent := lookupAgent(parentAgentID)
			// 预占位运行通知间隔，避免 spawn 后立即触发 running 通知
			// 新 subagent 出现时重置 continueSent，确保新一轮任务完成后能再次触发自动继续
			w.mu.Lock()
			w.lastSubagentNotify[parentAgentID] = time.Now()
			delete(w.continueSent, parentAgentID)
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
		w.mu.Lock()
		w.lastSubagentNotify = make(map[string]time.Time)
		w.continueSent = make(map[string]time.Time)
		w.mu.Unlock()
	})
}

// autoContinueRetryWindow 子任务完成后等待 agent 变为 idle 的最大重试时长
const autoContinueRetryWindow = 10 * time.Second

// autoContinueRetryInterval 重试检测的轮询间隔
const autoContinueRetryInterval = 5 * time.Second

// triggerAutoContinueAfterSubagents 子任务全部完成后触发主 agent 自动继续。
// 若父 agent 当前处于 idle/finished 则立即发送；若仍在 running 则启动 goroutine
// 在 autoContinueRetryWindow 内持续检测，agent 变为 idle 后发送；超时则放弃。
func (w *Watcher) triggerAutoContinueAfterSubagents(parentAgentID string, agent AgentStatus) {
	// 节流检查：距上次发送未超过 continueInterval 则跳过
	now := time.Now()
	w.mu.Lock()
	if lastSent, exists := w.continueSent[parentAgentID]; exists && now.Sub(lastSent) < w.continueInterval {
		w.mu.Unlock()
		logging.Infof("auto continue throttled agentId=%s remaining=%s", agent.ShortID, w.continueInterval-now.Sub(lastSent))
		return
	}
	w.mu.Unlock()

	// 立即发送条件
	if agent.Status == "idle" || (agent.AttentionReason != nil && *agent.AttentionReason == "finished") {
		logging.Infof("auto continue after subagents agentId=%s status=%s", agent.ShortID, agent.Status)
		w.doAutoContinue(parentAgentID, agent)
		return
	}

	// running 状态：启动延迟重试 goroutine
	if agent.Status == "running" {
		logging.Infof("auto continue deferred agentId=%s status=running timeout=%s", agent.ShortID, autoContinueRetryWindow)
		go w.retryAutoContinue(parentAgentID, agent.ShortID)
		return
	}

	// 其他状态（archived/unknown 等）：不发
	logging.Infof("auto continue skipped agentId=%s status=%s", agent.ShortID, agent.Status)
}

// retryAutoContinue 在 autoContinueRetryWindow 内持续检测 agent 状态，
// 变为 idle 后立即发送 continue 提示；超时/ctx 退出/新 subagent 出现则放弃。
func (w *Watcher) retryAutoContinue(parentAgentID string, shortID string) {
	ticker := time.NewTicker(autoContinueRetryInterval)
	defer ticker.Stop()
	timeout := time.After(autoContinueRetryWindow)

	for {
		select {
		case <-w.ctx.Done():
			logging.Infof("auto continue retry cancelled (watcher stopping) agentId=%s", shortID)
			return
		case <-timeout:
			logging.Warnf("auto continue retry timeout agentId=%s", shortID)
			return
		case <-ticker.C:
			// 检查是否有新 subagent 出现，有则放弃本次重试
			if w.subagentTracker != nil && w.subagentTracker.HasRunningSubagents(parentAgentID) {
				logging.Infof("auto continue retry aborted (new subagents running) agentId=%s", shortID)
				return
			}

			agent := w.fetchAgentStatus(parentAgentID)
			if agent == nil {
				// fetch 失败（网络抖动/断连等）不放弃，等下一轮 tick 重试
				logging.Warnf("auto continue retry fetch failed agentId=%s, will retry", shortID)
				continue
			}
			if agent.Status == "idle" || (agent.AttentionReason != nil && *agent.AttentionReason == "finished") {
				logging.Infof("auto continue retry success agentId=%s status=%s", shortID, agent.Status)
				w.doAutoContinue(parentAgentID, *agent)
				return
			}
			if agent.ArchivedAt != nil {
				logging.Infof("auto continue retry agent archived agentId=%s", shortID)
				return
			}
		}
	}
}

// doAutoContinue 执行实际的 continue prompt 发送和通知
func (w *Watcher) doAutoContinue(parentAgentID string, agent AgentStatus) {
	if err := w.continueAgent(parentAgentID, w.subagentDoneContinuePrompt); err != nil {
		logging.Warnf("auto continue after subagents failed agentId=%s err=%v", agent.ShortID, err)
		return
	}
	// 成功后记录时间戳，启动节流
	w.mu.Lock()
	w.continueSent[parentAgentID] = time.Now()
	w.mu.Unlock()

	acEv := AgentEvent{
		Type:      EventAutoContinue,
		Agent:     agent,
		Timestamp: time.Now(),
	}
	if err := w.notifier.Notify(w.ctx, acEv); err != nil {
		logging.Errorf("notify auto continue after subagents failed agentId=%s err=%v", parentAgentID, err)
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
	if !w.isEventEnabled(EventDisconnect) {
		logging.Debug("disconnect notification disabled by config")
		return
	}
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(true, w.daemonURL)
	}
}

func (w *Watcher) sendReconnectedNotify() {
	logging.Info("mcp daemon reconnected, agent notifications resumed")
	if !w.isEventEnabled(EventReconnect) {
		logging.Debug("reconnect notification disabled by config")
		return
	}
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(false, w.daemonURL)
	}
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

// lookupAgent 查找 agent 信息（fetchAgents 失败时返回仅有 ID 的占位 AgentStatus）
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
