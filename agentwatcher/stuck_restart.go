package agentwatcher

import (
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// ──────────────────────────────────────────────────────────
// 卡死自动重启 — Phase 2（从 runStuckCheck Step 5 抽出）
// ──────────────────────────────────────────────────────────

// checkRecoveryBeforeRestart 在自动重启前检查 Agent 是否已自行恢复
// 等待 stuckRestartDelay 后获取最新活动记录，与总超时对比
func (w *Watcher) checkRecoveryBeforeRestart(agent AgentStatus) bool {
	if w.stuckRestartDelay == 0 {
		return false
	}
	select {
	case <-time.After(w.stuckRestartDelay):
	case <-w.ctx.Done():
		return true
	}
	if w.ctx.Err() != nil {
		return true
	}

	entries := w.getAgentActivity(agent.ID)
	lastActivityTime := w.lastActivityTime(entries)

	totalTimeout := w.stuckDetectTimeout + w.stuckRestartDelay
	if lastActivityTime != nil && time.Since(*lastActivityTime) < totalTimeout {
		logging.Debugf("agent recovered after stuck notification agentId=%s", agent.ShortID)
		w.muRun(func() {
			if p := w.prevAgents[agent.ID]; p != nil {
				resetStuckState(p)
			}
		})
		return true
	}
	return false
}

// tryRestartAgent 执行一次 Agent 重启尝试，包含重试计数和上限判断
func (w *Watcher) tryRestartAgent(agent AgentStatus) bool {
	var exhausted bool
	var didSend bool
	w.muRun(func() {
		prev := w.prevAgents[agent.ID]
		if prev == nil {
			exhausted = true
			return
		}
		prev.RetryCount++
		if prev.RetryCount >= w.maxRetries {
			prev.StuckActionTaken = true
			exhausted = true
			logging.Warnf("agent restart retries exhausted agentId=%s retry=%d/%d",
				agent.ShortID, prev.RetryCount, w.maxRetries)
			return
		}
		logging.Warnf("agent restart retry %d/%d agentId=%s title=%s",
			prev.RetryCount, w.maxRetries, agent.ShortID, agent.Title)
		if err := w.restartAgent(agent.ID); err != nil {
			logging.Errorf("restart retry failed agentId=%s err=%v", agent.ID, err)
		} else {
			didSend = true
		}
		resetStuckState(prev)
	})
	if didSend {
		ev := AgentEvent{
			Type:      EventStuckContinue,
			Agent:     agent,
			Timestamp: time.Now(),
		}
		if err := w.notifier.Notify(w.ctx, ev); err != nil {
			logging.Errorf("notify stuck continue failed agentId=%s err=%v", agent.ID, err)
		} else {
			logging.Infof("stuck continue notified agentId=%s", agent.ShortID)
		}
	}
	return exhausted
}

// ──────────────────────────────────────────────────────────
// 卡死通知辅助函数（Phase 1 各步骤）
// ──────────────────────────────────────────────────────────

// sendStuckWarning 尝试发送疑似卡死警告通知（仅首次）
func (w *Watcher) sendStuckWarning(agent AgentStatus, now time.Time, idleDuration time.Duration) {
	w.muRun(func() {
		prev := w.prevAgents[agent.ID]
		if prev == nil || !prev.StuckChecking || prev.StuckWarningSent {
			return
		}
		prev.StuckWarningSent = true
	})
	if prev := w.getPrev(agent.ID); prev != nil && prev.StuckWarningSent {
		ev := AgentEvent{
			Type:         EventStuckWarning,
			Agent:        agent,
			Timestamp:    now,
			IdleDuration: idleDuration,
		}
		if err := w.notifier.Notify(w.ctx, ev); err != nil {
			logging.Errorf("notify stuck warning failed agentId=%s err=%v", agent.ID, err)
		} else {
			logging.Infof("stuck warning sent agentId=%s idle=%s", agent.ShortID, idleDuration)
		}
	}
}

// judgeStuckActivity 根据活动记录判断 Agent 是否仍在活跃
// lastActivityTime 为 nil 时（活动时间戳无法解析），保守视为仍在活跃，避免误判卡死
func (w *Watcher) judgeStuckActivity(agent AgentStatus, lastActivityTime *time.Time, now time.Time) bool {
	var stillActive bool
	w.muRun(func() {
		prev := w.prevAgents[agent.ID]
		if prev == nil || !prev.StuckChecking {
			return
		}
		if lastActivityTime == nil {
			// 无法获取活动时间戳，保守视为仍在活跃，不误判卡死
			stillActive = true
		} else if now.Sub(*lastActivityTime) < w.stuckDetectTimeout {
			stillActive = true
		} else {
			prev.StuckNotified = true
		}
	})
	return stillActive
}

// handleStillActive Agent 仍然活跃：发通知 + 重置卡死状态
func (w *Watcher) handleStillActive(agent AgentStatus, now time.Time, idleDuration time.Duration, entries []ActivityEntry) {
	if prev := w.getPrev(agent.ID); prev != nil && !prev.StillActiveNotified {
		w.muRun(func() {
			if p := w.prevAgents[agent.ID]; p != nil {
				p.StillActiveNotified = true
			}
		})
		ev := AgentEvent{
			Type:            EventStillActive,
			Agent:           agent,
			Timestamp:       now,
			ActivityEntries: entries,
			IdleDuration:    idleDuration,
		}
		if err := w.notifier.Notify(w.ctx, ev); err != nil {
			logging.Errorf("notify still active failed agentId=%s err=%v", agent.ID, err)
		} else {
			logging.Infof("still active notified agentId=%s entries=%d", agent.ShortID, len(entries))
		}
	}
	w.muRun(func() {
		if p := w.prevAgents[agent.ID]; p != nil {
			resetStuckState(p)
		}
	})
}

// sendStuckNotification 发送确认卡死通知
func (w *Watcher) sendStuckNotification(agent AgentStatus, now time.Time, entries []ActivityEntry) {
	if prev := w.getPrev(agent.ID); prev != nil && prev.StuckNotified {
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
}

// ──────────────────────────────────────────────────────────
// 通用辅助函数（包内共享）
// ──────────────────────────────────────────────────────────

// muRun 在 w.mu 保护下执行 fn，用于 goroutine 中安全更新 AgentState
func (w *Watcher) muRun(fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fn()
}

// getPrev 线程安全地读取 prevAgents[agentID]
func (w *Watcher) getPrev(agentID string) *AgentState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.prevAgents[agentID]
}

// resetStuckState 重置卡死检测全部状态标记（保留 RetryCount）
func resetStuckState(prev *AgentState) {
	prev.StuckSince = ""
	prev.StuckNotified = false
	prev.StuckActionTaken = false
	prev.StuckWarningSent = false
	prev.StillActiveNotified = false
	prev.StuckChecking = false
}

// detectNewPermission 检测新的权限请求，通知异步发送不阻塞主循环
func (w *Watcher) detectNewPermission(perm PermissionRequest) {
	key := perm.AgentID + "/" + perm.Request.ID
	if w.prevPermIDs[key] {
		return
	}
	w.prevPermIDs[key] = true

	if perm.Status != "running" {
		return
	}

	go func() {
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
	}()
}