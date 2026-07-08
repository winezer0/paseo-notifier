package agentwatcher

import (
	"strings"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// detectAgentChange 检测 Agent 的 attentionReason 变更（finished/error）
func (w *Watcher) detectAgentChange(agent AgentStatus) {
	w.mu.Lock()
	prev, exists := w.prevAgents[agent.ID]
	if !exists {
		w.prevAgents[agent.ID] = &AgentState{
			AttentionReason:    agent.AttentionReason,
			AttentionTimestamp: agent.AttentionTimestamp,
			LastUpdatedAt:      agent.UpdatedAt,
		}
		w.mu.Unlock()
		return
	}

	if agent.ArchivedAt != nil {
		delete(w.prevAgents, agent.ID)
		// 同时清理该 Agent 的权限请求记录，防止 prevPermIDs 无限增长
		for key := range w.prevPermIDs {
			if strings.HasPrefix(key, agent.ID+"/") {
				delete(w.prevPermIDs, key)
			}
		}
		w.mu.Unlock()
		return
	}

	// 正在被 goroutine 异步处理，跳过状态变更以防并发冲突
	if prev.StuckChecking {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	var eventType EventType

	if agent.AttentionReason != nil {
		trigger := false

		if prev.AttentionReason == nil {
			trigger = true
		} else if *prev.AttentionReason != *agent.AttentionReason {
			trigger = true
		} else if !ptrTimeEqual(prev.AttentionTimestamp, agent.AttentionTimestamp) {
			trigger = true
		}

		if trigger {
			switch *agent.AttentionReason {
			case "finished":
				eventType = EventFinished
			case "error":
				eventType = EventError
			}
		}
	}

	if eventType != "" {
		// 在发通知前更新状态快照，解锁后 HTTP/通知不持锁
		w.mu.Lock()
		w.prevAgents[agent.ID] = &AgentState{
			AttentionReason:    agent.AttentionReason,
			AttentionTimestamp: agent.AttentionTimestamp,
			LastUpdatedAt:      agent.UpdatedAt,
		}
		w.mu.Unlock()

		// 获取活动摘要，附加到通知中
		activityEntries := w.getAgentActivity(agent.ID)

		ev := AgentEvent{
			Type:            eventType,
			Agent:           agent,
			Timestamp:       time.Now(),
			ActivityEntries: activityEntries,
		}
		if err := w.notifier.Notify(w.ctx, ev); err != nil {
			logging.Errorf("notify failed event=%s agentId=%s err=%v", eventType, agent.ID, err)
		} else {
			logging.Infof("agent event detected event=%s agentId=%s title=%s entries=%d", eventType, agent.ShortID, agent.Title, len(activityEntries))
		}
		// event 触发后 prev 指针已指向新快照，不再更新旧指针
		return
	}

	prev.AttentionReason = agent.AttentionReason
	prev.AttentionTimestamp = agent.AttentionTimestamp
}

// ──────────────────────────────────────────────────────────
// 卡死检测 — 主循环（同步，仅 Phase 0）
// ──────────────────────────────────────────────────────────

// detectStuckAgents 主循环，只做 Phase 0（UpdatedAt 追踪），
// 超时后启动后台 goroutine 处理后续的 HTTP 检查和通知，
// 主循环不等待、不阻塞。
func (w *Watcher) detectStuckAgents(agents []AgentStatus) {
	if w.stuckDetectTimeout == 0 {
		return
	}
	now := time.Now()
	for _, agent := range agents {
		if w.shouldSkipStuckCheck(agent) {
			continue
		}
		prev, exists := w.prevAgents[agent.ID]
		if !exists {
			continue
		}

		// Phase 0：UpdatedAt 变化追踪（纯内存操作，无 IO）
		handled, stuckSince := w.stuckPhase0(agent, prev, now)
		if handled {
			continue
		}

		// 超时 + 没有正在运行的 goroutine → 启动异步卡死检查
		w.mu.Lock()
		if !prev.StuckNotified && now.Sub(*stuckSince) >= w.stuckDetectTimeout && !prev.StuckChecking {
			prev.StuckChecking = true
			w.mu.Unlock()
			go w.runStuckCheck(agent, *stuckSince)
			continue
		}
		w.mu.Unlock()
	}
}

// ──────────────────────────────────────────────────────────
// 运行中状态心跳通知
// ──────────────────────────────────────────────────────────

// checkRunningAgents 检查运行中的 Agent，当静止时间超过间隔时发送状态心跳通知
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

		// 计算静止时长：以 UpdatedAt 和 LastUserMessageAt 中较新的为准
		lastActive := agent.UpdatedAt
		if agent.LastUserMessageAt != "" {
			if agent.UpdatedAt == "" || agent.LastUserMessageAt > agent.UpdatedAt {
				lastActive = agent.LastUserMessageAt
			}
		}
		lastActiveTime, err := time.Parse(time.RFC3339, lastActive)
		if err != nil || lastActive == "" {
			continue
		}
		silence := now.Sub(lastActiveTime)
		if silence < w.runningStatusInterval {
			continue
		}

		// 检查上次通知间隔
		w.mu.Lock()
		if prev.LastRunningNotify != nil && now.Sub(*prev.LastRunningNotify) < w.runningStatusInterval {
			w.mu.Unlock()
			continue
		}
		prev.LastRunningNotify = &now
		w.mu.Unlock()

		// 发送通知（异步获取活动记录）
		go func(agent AgentStatus) {
			entries := w.getAgentActivity(agent.ID)
			ev := AgentEvent{
				Type:            EventRunningStatus,
				Agent:           agent,
				Timestamp:       now,
				ActivityEntries: entries,
			}
			if err := w.notifier.Notify(w.ctx, ev); err != nil {
				logging.Errorf("notify running status failed agentId=%s err=%v", agent.ID, err)
			} else {
				logging.Infof("running status notified agentId=%s title=%s silence=%s", agent.ShortID, agent.Title, silence)
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
	//已归档的 Agent 不再活动，不需要监控
	case agent.ArchivedAt != nil:
		return true
	//	Agent 不在运行中（状态为 idle 等）
	case agent.Status != "running": // idle 为等待用户输入，不检测
		return true
	//	已经结束的任务，UpdatedAt 自然不会再更新
	case agent.AttentionReason != nil && (*agent.AttentionReason == "finished" || *agent.AttentionReason == "error"):
		return true
	//	w.managedAgents 非空且当前 Agent 不在白名单中
	case len(w.managedAgents) > 0 && !w.managedAgents[agent.ID]:
		return true
	}
	return false
}

// ──────────────────────────────────────────────────────────
// Phase 0 - UpdatedAt 变化追踪
// ──────────────────────────────────────────────────────────

// stuckPhase0 追踪 UpdatedAt 变化，返回 (是否已处理, 解析后的卡死起始时间)
//
//	handled=true  → 本轮已处理完毕，外层应 continue
//	handled=false → stuckSince 有效，超时后可启动异步检查
func (w *Watcher) stuckPhase0(agent AgentStatus, prev *AgentState, now time.Time) (handled bool, stuckSince *time.Time) {
	switch {
	case prev.StuckChecking:
		// 已有后台 goroutine 在处理，主循环跳过它
		return true, nil

	case agent.UpdatedAt == "":
		//MCP 守护进程没返回更新时间，无法判断 Agent 活动状态
		//重置全部卡死标记 + RetryCount，本轮跳过
		resetStuckState(prev)
		prev.RetryCount = 0
		return true, nil

	case agent.UpdatedAt != prev.LastUpdatedAt:
		//当前 UpdatedAt 跟上次记录的值不同
		//Agent 的 UpdatedAt 还在正常推进 → Agent 正在工作中
		//更新 LastUpdatedAt，重置全部卡死标记 + RetryCount，本轮跳过
		prev.LastUpdatedAt = agent.UpdatedAt
		resetStuckState(prev)
		prev.RetryCount = 0
		return true, nil

	case prev.StuckSince == "":
		//UpdatedAt 跟上次一样（没变化），且之前没记录过卡死起始时间
		//记录当前时间到 StuckSince，本轮跳过，等下一轮看是否超时
		prev.StuckSince = now.Format(time.RFC3339)
		return true, nil

	case prev.StuckActionTaken:
		//已经执行过 "自动重启" 的操作
		//所有重试次数已用完，不再做任何处理
		//直接跳过，不再进入 Phase 1/2
		return true, nil
	}

	//之前记录的 StuckSince 不是合法的 RFC3339 格式
	//数据异常（极少发生） 重置状态，让下一轮重新记录
	t, err := time.Parse(time.RFC3339, prev.StuckSince)
	if err != nil {
		resetStuckState(prev)
		return true, nil
	}
	return false, &t
}

// ──────────────────────────────────────────────────────────
// 后台 goroutine — 完整的卡死检查流程
// ──────────────────────────────────────────────────────────

// runStuckCheck 在后台 goroutine 中完整执行 Phase 1（预警→二次确认）
// 和 Phase 2（自动重启），主循环不等待它完成。
func (w *Watcher) runStuckCheck(agent AgentStatus, stuckSince time.Time) {
	defer w.muRun(func() {
		if prev := w.prevAgents[agent.ID]; prev != nil {
			prev.StuckChecking = false
		}
	})

	if w.ctx.Err() != nil {
		return
	}

	now := time.Now()
	idleDuration := now.Sub(stuckSince)

	// ── Step 1：发送疑似卡死警告（仅一次） ──
	w.sendStuckWarning(agent, now, idleDuration)

	if w.ctx.Err() != nil {
		return
	}

	// ── Step 2：获取活动记录，二次确认 ──
	entries := w.getAgentActivity(agent.ID)
	lastActivityTime := w.lastActivityTime(entries)

	if w.ctx.Err() != nil {
		return
	}

	// ── Step 3：判断结果 ──
	if w.judgeStuckActivity(agent, lastActivityTime, now) {
		w.handleStillActive(agent, now, idleDuration, entries)
		return
	}

	// ── Step 4：确认卡死，发通知 ──
	w.sendStuckNotification(agent, now, entries)

	// ── Step 5：Phase 2 自动重启（仅 stuckRestartDelay>0） ──
	if w.stuckRestartDelay == 0 {
		return
	}
	if w.checkRecoveryBeforeRestart(agent) {
		return
	}
	w.tryRestartAgent(agent)
}

// ──────────────────────────────────────────────────────────
// 过滤
// ──────────────────────────────────────────────────────────
