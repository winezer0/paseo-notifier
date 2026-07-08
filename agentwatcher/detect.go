package agentwatcher

import (
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// detectAgentChange 检测 Agent 的 attentionReason 变更（finished/error）
func (w *Watcher) detectAgentChange(agent AgentStatus) {
	prev, exists := w.prevAgents[agent.ID]
	if !exists {
		w.prevAgents[agent.ID] = &AgentState{
			AttentionReason:    agent.AttentionReason,
			AttentionTimestamp: agent.AttentionTimestamp,
			LastUpdatedAt:      agent.UpdatedAt,
		}
		return
	}

	if agent.ArchivedAt != nil {
		return
	}

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
		w.prevAgents[agent.ID] = &AgentState{
			AttentionReason:    agent.AttentionReason,
			AttentionTimestamp: agent.AttentionTimestamp,
			LastUpdatedAt:      agent.UpdatedAt,
		}

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
	}

	prev.AttentionReason = agent.AttentionReason
	prev.AttentionTimestamp = agent.AttentionTimestamp
}

// ──────────────────────────────────────────────────────────
// 卡死检测主流程
// ──────────────────────────────────────────────────────────

// detectStuckAgents 卡死检测主循环，对每个 Agent 依次执行 Phase 0→1→2
//
//	Phase 0 - UpdatedAt 变化追踪：停更则记录 StuckSince，恢复则重置
//	Phase 1 - 超时确认 + 预警：stuckDetectTimeout 到后发警告→二次确认→卡死/活动通知
//	Phase 2 - 自动恢复：stuckRestartDelay 后尝试重启，超 maxRetries 次放弃
//
//	stuckDetectTimeout=0 时整个检测禁用
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
		// Phase 0 - UpdatedAt 变化追踪
		handled, stuckSince := w.stuckPhase0(agent, prev, now)
		if handled {
			continue
		}
		if !prev.StuckNotified {
			// stuckPhase1 执行 Phase 1 三步操作：超时判断 → 发警告 → 活动确认
			w.stuckPhase1(agent, prev, now, *stuckSince)
			continue // Phase 1 在本轮始终结束迭代
		}
		// stuckPhase2 执行 Phase 2 自动重启逻辑（仅 stuckRestartDelay>0 时启用）
		w.stuckPhase2(agent, prev, now, *stuckSince)
	}
}

// ──────────────────────────────────────────────────────────
// 过滤
// ──────────────────────────────────────────────────────────

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
//	handled=false → stuckSince 有效，可传入 Phase 1/2
func (w *Watcher) stuckPhase0(agent AgentStatus, prev *AgentState, now time.Time) (handled bool, stuckSince *time.Time) {
	switch {
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
// Phase 1 - 超时确认 + 预警 + 二次确认
// ──────────────────────────────────────────────────────────

// stuckPhase1 执行 Phase 1 三步操作：超时判断 → 发警告 → 活动确认
func (w *Watcher) stuckPhase1(agent AgentStatus, prev *AgentState, now time.Time, stuckSince time.Time) {
	if now.Sub(stuckSince) < w.stuckDetectTimeout {
		return
	}
	w.sendStuckWarning(agent, prev, now, stuckSince)

	entries := w.getAgentActivity(agent.ID)
	lastActivityTime := w.lastActivityTime(entries)
	if lastActivityTime != nil && now.Sub(*lastActivityTime) < w.stuckDetectTimeout {
		w.sendStillActiveNotify(agent, prev, now, stuckSince, entries)
		return
	}
	w.confirmStuck(agent, prev, now, entries)
}

// sendStuckWarning 发送疑似卡死警告，每个卡死周期仅一次
func (w *Watcher) sendStuckWarning(agent AgentStatus, prev *AgentState, now time.Time, stuckSince time.Time) {
	if prev.StuckWarningSent {
		return
	}
	prev.StuckWarningSent = true
	idleDuration := now.Sub(stuckSince)
	ev := AgentEvent{
		Type:         EventStuckWarning,
		Agent:        agent,
		Timestamp:    now,
		IdleDuration: idleDuration,
	}
	if err := w.notifier.Notify(w.ctx, ev); err != nil {
		logging.Errorf("notify stuck warning failed agentId=%s err=%v", agent.ID, err)
	}
	logging.Infof("stuck warning sent agentId=%s idle=%s", agent.ShortID, idleDuration)
}

// sendStillActiveNotify 发送活动正常通知，重置卡死状态
func (w *Watcher) sendStillActiveNotify(agent AgentStatus, prev *AgentState, now time.Time, stuckSince time.Time, entries []ActivityEntry) {
	if !prev.StillActiveNotified {
		prev.StillActiveNotified = true
		idleDuration := now.Sub(stuckSince)
		ev := AgentEvent{
			Type:            EventStillActive,
			Agent:           agent,
			Timestamp:       now,
			ActivityEntries: entries,
			IdleDuration:    idleDuration,
		}
		if err := w.notifier.Notify(w.ctx, ev); err != nil {
			logging.Errorf("notify still active failed agentId=%s err=%v", agent.ID, err)
		}
		logging.Infof("still active notified agentId=%s entries=%d", agent.ShortID, len(entries))
	}
	resetStuckState(prev)
}

// confirmStuck 确认卡死，发送 EventStuck 通知
func (w *Watcher) confirmStuck(agent AgentStatus, prev *AgentState, now time.Time, entries []ActivityEntry) {
	prev.StuckNotified = true
	logging.Warnf("agent may be stuck agentId=%s title=%s idleSince=%s", agent.ShortID, agent.Title, prev.StuckSince)
	ev := AgentEvent{
		Type:            EventStuck,
		Agent:           agent,
		Timestamp:       now,
		ActivityEntries: entries,
	}
	if err := w.notifier.Notify(w.ctx, ev); err != nil {
		logging.Errorf("notify stuck failed agentId=%s err=%v", agent.ID, err)
	}
}

// ──────────────────────────────────────────────────────────
// Phase 2 - 自动重启
// ──────────────────────────────────────────────────────────

// stuckPhase2 执行 Phase 2 自动重启逻辑（仅 stuckRestartDelay>0 时启用）
func (w *Watcher) stuckPhase2(agent AgentStatus, prev *AgentState, now time.Time, stuckSince time.Time) {
	if w.stuckRestartDelay == 0 {
		return
	}
	totalTimeout := w.stuckDetectTimeout + w.stuckRestartDelay
	if now.Sub(stuckSince) < totalTimeout {
		return
	}

	entries := w.getAgentActivity(agent.ID)
	lastActivityTime := w.lastActivityTime(entries)
	if lastActivityTime != nil && now.Sub(*lastActivityTime) < totalTimeout {
		logging.Debugf("agent recovered after stuck notification agentId=%s", agent.ShortID)
		resetStuckState(prev)
		return
	}
	w.executeStuckRestart(agent, prev)
}

// executeStuckRestart 执行重启重试，超 maxRetries 次后放弃
func (w *Watcher) executeStuckRestart(agent AgentStatus, prev *AgentState) {
	prev.RetryCount++
	if prev.RetryCount >= w.maxRetries {
		prev.StuckActionTaken = true
		logging.Warnf("agent restart retries exhausted agentId=%s retry=%d/%d",
			agent.ShortID, prev.RetryCount, w.maxRetries)
		return
	}
	logging.Warnf("agent restart retry %d/%d agentId=%s title=%s",
		prev.RetryCount, w.maxRetries, agent.ShortID, agent.Title)
	if err := w.restartAgent(agent.ID); err != nil {
		logging.Errorf("restart retry failed agentId=%s err=%v", agent.ID, err)
	}
	resetStuckState(prev)
}

// ──────────────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────────────

// resetStuckState 重置卡死检测全部状态标记（保留 RetryCount）
func resetStuckState(prev *AgentState) {
	prev.StuckSince = ""
	prev.StuckNotified = false
	prev.StuckActionTaken = false
	prev.StuckWarningSent = false
	prev.StillActiveNotified = false
}

// detectNewPermission 检测新的权限请求
func (w *Watcher) detectNewPermission(perm PermissionRequest) {
	key := perm.AgentID + "/" + perm.Request.ID
	if w.prevPermIDs[key] {
		return
	}
	w.prevPermIDs[key] = true

	if perm.Status != "running" {
		return
	}

	ev := AgentEvent{
		Type:       EventPermissionRequest,
		Timestamp:  time.Now(),
		Permission: &perm,
		Agent: AgentStatus{
			ID:       perm.AgentID,
			Provider: perm.Request.Provider,
		},
	}

	if err := w.notifier.Notify(w.ctx, ev); err != nil {
		logging.Errorf("notify permission failed agentId=%s kind=%s err=%v", perm.AgentID, perm.Request.Kind, err)
	} else {
		logging.Infof("permission request detected agentId=%s kind=%s title=%s", perm.AgentID, perm.Request.Kind, perm.Request.Title)
	}
}
