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

// detectStuckAgents 检查运行中的 Agent 是否有 UpdatedAt 长期无变化（卡死）
// stuckDetectTimeout=0 时禁用卡死检测
// stuckRestartDelay>0 时检测到卡死后延迟指定秒数自动重启
func (w *Watcher) detectStuckAgents(agents []AgentStatus) {
	if w.stuckDetectTimeout == 0 {
		return
	}
	now := time.Now()
	for _, agent := range agents {
		if agent.ArchivedAt != nil {
			continue
		}
		// 已经 finished/error 的不需要检查卡死，但处理中的 Agent 仍可能卡死
		if agent.AttentionReason != nil && (*agent.AttentionReason == "finished" || *agent.AttentionReason == "error") {
			continue
		}
		// 如果设置了 managedAgents，只处理白名单内的 Agent
		if len(w.managedAgents) > 0 && !w.managedAgents[agent.ID] {
			continue
		}

		prev, exists := w.prevAgents[agent.ID]
		if !exists {
			continue
		}

		// UpdatedAt 为空或已变化 → 重置卡死状态
		if agent.UpdatedAt == "" {
			prev.StuckSince = ""
			prev.StuckNotified = false
			prev.StuckActionTaken = false
			prev.RetryCount = 0
			continue
		}
		if agent.UpdatedAt != prev.LastUpdatedAt {
			prev.LastUpdatedAt = agent.UpdatedAt
			prev.StuckSince = ""
			prev.StuckNotified = false
			prev.StuckActionTaken = false
			prev.RetryCount = 0
			continue
		}

		// UpdatedAt 无变化 — 可能卡死
		if prev.StuckSince == "" {
			prev.StuckSince = now.Format(time.RFC3339)
			continue
		}

		if prev.StuckActionTaken {
			continue
		}

		stuckSince, err := time.Parse(time.RFC3339, prev.StuckSince)
		if err != nil {
			prev.StuckSince = ""
			prev.StuckNotified = false
			prev.StuckActionTaken = false
			continue
		}

		if !prev.StuckNotified {
			// 第一阶段：stuckDetectTimeout 达到，发送卡死通知
			if now.Sub(stuckSince) < w.stuckDetectTimeout {
				continue
			}

			entries := w.getAgentActivity(agent.ID)
			lastActivityTime := w.lastActivityTime(entries)
			if lastActivityTime != nil && now.Sub(*lastActivityTime) < w.stuckDetectTimeout {
				logging.Debugf("agent still active agentId=%s", agent.ShortID)
				prev.StuckSince = ""
				prev.StuckNotified = false
				prev.StuckActionTaken = false
				continue
			}

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
			continue
		}

		// 第二阶段：stuckRestartDelay 后自动重启
		if w.stuckRestartDelay == 0 {
			continue
		}
		if now.Sub(stuckSince) < w.stuckDetectTimeout+w.stuckRestartDelay {
			continue
		}

		entries := w.getAgentActivity(agent.ID)
		lastActivityTime := w.lastActivityTime(entries)
		if lastActivityTime != nil && now.Sub(*lastActivityTime) < w.stuckDetectTimeout+w.stuckRestartDelay {
			logging.Debugf("agent recovered after stuck notification agentId=%s", agent.ShortID)
			prev.StuckSince = ""
			prev.StuckNotified = false
			prev.StuckActionTaken = false
			continue
		}

		// 确认卡死，重试或放弃
		prev.RetryCount++
		if prev.RetryCount >= w.maxRetries {
			prev.StuckActionTaken = true
			logging.Warnf("agent restart retries exhausted agentId=%s retry=%d/%d",
				agent.ShortID, prev.RetryCount, w.maxRetries)
		} else {
			logging.Warnf("agent restart retry %d/%d agentId=%s title=%s",
				prev.RetryCount, w.maxRetries, agent.ShortID, agent.Title)
			if err := w.restartAgent(agent.ID); err != nil {
				logging.Errorf("restart retry failed agentId=%s err=%v", agent.ID, err)
			}
			prev.StuckSince = ""
			prev.StuckNotified = false
			prev.StuckActionTaken = false
		}
	}
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
		Type:      EventPermissionRequest,
		Timestamp: time.Now(),
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