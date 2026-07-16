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
			case "cancelled":
				eventType = EventCancelled
			default:
				logging.Warnf("unknown attentionReason=%q agentId=%s, treating as finished", *agent.AttentionReason, agent.ShortID)
				eventType = EventFinished
			}
		}
	}

	if eventType != "" {
		// 已完成但还有子任务在运行 → 父 agent 只是委派了工作，不算真正完成
		if eventType == EventFinished && w.subagentTracker != nil && w.subagentTracker.HasRunningSubagents(agent.ID) {
			logging.Infof("agent finished but has running subagents, suppressing notification agentId=%s", agent.ShortID)
			// 仍更新状态快照，否则下次轮询会重复检测
			w.updateAgentSnapshot(agent, prev)
			return
		}

		// 短任务抑制：完成时长短于阈值的 finished 任务不通知（用户通常已看到结果）
		// 使用上次 UpdatedAt 到当前的时间差作为任务耗时（反映用户感知的执行时长）
		if eventType == EventFinished && w.notifyMinDuration > 0 {
			if prev := w.getPrev(agent.ID); prev != nil && prev.LastUpdatedAt != "" {
				if lastUpdate, err := time.Parse(time.RFC3339, prev.LastUpdatedAt); err == nil {
					if idle := time.Since(lastUpdate); idle < w.notifyMinDuration {
					logging.Infof("agent finished within notify_min_duration, suppressing notification agentId=%s idle=%s",
						agent.ShortID, idle)
					w.updateAgentSnapshot(agent, prev)
					return
					}
				}
			}
		}

		// 在发通知前更新状态快照，解锁后 HTTP/通知不持锁
		w.updateAgentSnapshot(agent, prev)

		// 用户取消/手动终止：仅记录日志，不发送通知
		if eventType == EventCancelled {
			logging.Infof("agent cancelled by user agentId=%s title=%s", agent.ShortID, agent.Title)
			return
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

		// 任务完成后自动继续：检查最后一条活动是否包含继续请求
		if eventType == EventFinished && w.autoContinueKeyword && w.continuePrompt != "" {
			if w.shouldAutoContinue(activityEntries, agent.ID) {
				logging.Infof("auto continue agentId=%s", agent.ShortID)
				if err := w.continueAgent(agent.ID, w.continuePrompt); err != nil {
					logging.Warnf("auto continue failed agentId=%s err=%v", agent.ShortID, err)
				} else {
					// 发送自动继续通知
					acEv := AgentEvent{
						Type:      EventAutoContinue,
						Agent:     agent,
						Timestamp: time.Now(),
					}
					if err := w.notifier.Notify(w.ctx, acEv); err != nil {
						logging.Errorf("notify auto continue failed agentId=%s err=%v", agent.ID, err)
					}
				}
			}
		}

		// event 触发后 prev 指针已指向新快照，不再更新旧指针
		return
	}

	prev.AttentionReason = agent.AttentionReason
	prev.AttentionTimestamp = agent.AttentionTimestamp
}

// updateAgentSnapshot 更新 Agent 状态快照，保留 LastRunningNotify 避免重复触发运行中状态通知
func (w *Watcher) updateAgentSnapshot(agent AgentStatus, prev *AgentState) {
	w.mu.Lock()
	lastRunningNotify := prev.LastRunningNotify
	w.prevAgents[agent.ID] = &AgentState{
		AttentionReason:    agent.AttentionReason,
		AttentionTimestamp: agent.AttentionTimestamp,
		LastUpdatedAt:      agent.UpdatedAt,
		LastRunningNotify:  lastRunningNotify,
	}
	w.mu.Unlock()
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

	// ── Step 0：重新获取 Agent 最新状态，确认仍 running ──
	// 避免 Agent 已在超时等待期间完成但仍收到误报警告
	latest := w.fetchAgentStatus(agent.ID)
	if latest != nil {
		agent = *latest
		if agent.ArchivedAt != nil || agent.Status != "running" ||
			(agent.AttentionReason != nil && (*agent.AttentionReason == "finished" || *agent.AttentionReason == "error")) {
			logging.Infof("agent no longer running, aborting stuck check agentId=%s status=%s reason=%v",
				agent.ShortID, agent.Status, agent.AttentionReason)
			return
		}
	}

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
// 自动继续检测
// ──────────────────────────────────────────────────────────

// continueKeywords 自动继续触发的关键词列表
var continueKeywordsZh = []string{"继续"}
var continueKeywordsEn = []string{"continue"}

// trailingColonRunes 触发自动继续的结尾冒号类型（可扩展）
var trailingColonRunes = []rune{':', '：'}

// 统一修剪符号集合（去重空白、中英文标点）
const allTrimSet = " \t\n\r！？。，；：“”‘’()（）[]【】、～·!?,.;:\"'{}<>-~`"

func (w *Watcher) shouldAutoContinue(entries []ActivityEntry, agentID string) bool {
	if len(entries) == 0 {
		return false
	}

	last := entries[len(entries)-1]
	summary := last.Summary
	if summary == "" {
		return false
	}

	// 冒号结尾检测：原始文本以冒号结尾（可能后跟空白），说明 Agent 输出被截断
	if hasTrailingColon(summary) {
		logging.Debugf("auto continue triggered by trailing colon agentId=%s summary=%q", agentID, summary)
		return true
	}

	// 第一步：全局统一去除首尾杂字符，只做一次，避免重复清理
	trim := strings.Trim(summary, allTrimSet)

	// 中文关键词：清理后文本取末尾5字符匹配
	if hit, kw, tail := matchTailKeywords(trim, 10, continueKeywordsZh); hit {
		logging.Debugf("auto continue triggered by zh keyword=%q tailZh=%q agentId=%s", kw, tail, agentID)
		return true
	}

	// 英文关键词：清理后文本取末尾20字符匹配
	if hit, kw, tail := matchTailKeywords(trim, 20, continueKeywordsEn); hit {
		logging.Debugf("auto continue triggered by en keyword=%q tailEn=%q agentId=%s", kw, tail, agentID)
		return true
	}

	return false
}

// matchTailKeywords 接收已清理干净的文本，截取尾部N字符并匹配关键词
func matchTailKeywords(cleanStr string, tailLen int, keywords []string) (hit bool, hitKw string, tail string) {
	tail = getTailRune(cleanStr, tailLen)
	for _, kw := range keywords {
		if strings.Contains(tail, kw) {
			return true, kw, tail
		}
	}
	return false, "", tail
}

// getTailRune 获取字符串尾部N个rune字符
func getTailRune(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// hasTrailingColon 检查文本去除尾部空格后是否以冒号结尾
// 用于检测 Agent 在列举/阐述时被截断的情况，如 "以下方案：" 或 "Here are the items:"
func hasTrailingColon(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	lastRune := []rune(trimmed)[len([]rune(trimmed))-1]
	for _, colon := range trailingColonRunes {
		if lastRune == colon {
			return true
		}
	}
	return false
}
