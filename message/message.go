package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
)

// currentLang 全局语言设置，由 SetLang 初始化
var currentLang Lang = LangEn

// SetLang 设置通知消息的全局语言
func SetLang(lang Lang) {
	currentLang = lang
}

// Build 根据事件类型构建通知主题和内容
func Build(event agentwatcher.AgentEvent) (subject, content string) {
	msg := getMessages(currentLang)
	switch event.Type {
	case agentwatcher.EventFinished:
		subject = msg.SubjectFinished
		content = buildFinishedContent(event, msg)
	case agentwatcher.EventError:
		subject = msg.SubjectError
		content = buildErrorContent(event, msg)
	case agentwatcher.EventPermissionRequest:
		subject = msg.SubjectPermission
		content = buildPermissionContent(event, msg)
	case agentwatcher.EventStuck:
		subject = msg.SubjectStuck
		content = buildStuckContent(event, msg)
	case agentwatcher.EventStuckWarning:
		subject = msg.SubjectStuckWarning
		content = buildStuckWarningContent(event, msg)
	case agentwatcher.EventStillActive:
		subject = msg.SubjectStillActive
		content = buildStillActiveContent(event, msg)
	case agentwatcher.EventRunningStatus:
		subject = msg.SubjectRunningStatus
		content = buildRunningStatusContent(event, msg)
	case agentwatcher.EventSubagentProgress:
		subject = msg.SubjectSubagentProgress
		content = buildSubagentProgressContent(event, msg)
	case agentwatcher.EventAllSubagentsDone:
		subject = msg.SubjectAllSubagentsDone
		content = buildAllSubagentsDoneContent(event, msg)
	case agentwatcher.EventSubagentSpawned:
		subject = msg.SubjectSubagentSpawned
		content = buildSubagentSpawnedContent(event, msg)
	case agentwatcher.EventSubagentRunning:
		subject = msg.SubjectSubagentRunning
		content = buildSubagentRunningContent(event, msg)
	case agentwatcher.EventAutoContinue:
		subject = msg.SubjectAutoContinue
		content = buildAutoContinueContent(event, msg)
	case agentwatcher.EventStuckContinue:
		subject = msg.SubjectStuckContinue
		content = buildStuckContinueContent(event, msg)
	}
	return
}

// buildFinishedContent 构建 Agent 任务完成通知内容
func buildFinishedContent(event agentwatcher.AgentEvent, msg messages) string {
	return buildAgentTimeContent(event, msg, msg.FieldTaskCompletedAt)
}

// buildErrorContent 构建 Agent 任务失败通知内容
func buildErrorContent(event agentwatcher.AgentEvent, msg messages) string {
	return buildAgentTimeContent(event, msg, msg.FieldTaskFailedAt)
}

// buildAgentInfoSection 构建 Agent 基本信息区块（标题、ID、模型、思考、目录）
// 两个通知构建函数（完成/失败、卡死）共用此区块
func buildAgentInfoSection(a agentwatcher.AgentStatus, msg messages) string {
	var b strings.Builder
	b.WriteString(msg.SectionAgent + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldTitle, a.Title))
	b.WriteString(fmt.Sprintf("%s: %s (%s)\n", msg.FieldAgent, a.ShortID, a.ID))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldModel, modelLabel(a)))
	if a.ThinkingOptionID != nil && *a.ThinkingOptionID != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldThinking, *a.ThinkingOptionID))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldDirectory, a.CWD))
	return b.String()
}

// buildAgentTimeContent 构建 Agent 完成或失败通知的时间内容
// timeLabel 为时间戳字段的标签（"完成时间" 或 "失败时间"）
func buildAgentTimeContent(event agentwatcher.AgentEvent, msg messages, timeLabel string) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionTime + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentCreatedAt, formatStrTime(a.CreatedAt)))
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentDuration, calcDuration(a.CreatedAt, a.AttentionTimestamp)))
	}
	if a.UpdatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentUpdatedAt, formatStrTime(a.UpdatedAt)))
	}
	startTime := a.CreatedAt
	if a.LastUserMessageAt != "" {
		startTime = a.LastUserMessageAt
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldLastUserAsk, formatStrTime(a.LastUserMessageAt)))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", timeLabel, formatTime(a.AttentionTimestamp)))
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldTaskDuration, calcDuration(startTime, a.AttentionTimestamp)))
	}

	if len(a.Labels) > 0 {
		b.WriteString("\n" + msg.SectionLabels + "\n")
		for k, v := range a.Labels {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	buildActivitySection(&b, event.ActivityEntries, msg)

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildStuckContent 构建 Agent 疑似卡死通知内容
func buildStuckContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionTime + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentCreatedAt, formatStrTime(a.CreatedAt)))
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentDuration, calcDuration(a.CreatedAt, nil)))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentUpdatedAt, formatStrTime(a.UpdatedAt)))
	if a.LastUserMessageAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldLastUserAsk, formatStrTime(a.LastUserMessageAt)))
	}

	if len(a.Labels) > 0 {
		b.WriteString("\n" + msg.SectionLabels + "\n")
		for k, v := range a.Labels {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	// 卡死通知附加卡死原因、空闲时长和最后活动摘要
	reason := stuckReason(event.ActivityEntries)
	if reason != "" {
		b.WriteString(fmt.Sprintf("\n%s: %s\n", msg.FieldStuckReason, reason))
	}

	lastEntry := lastActivityEntry(event.ActivityEntries)
	if lastEntry != nil {
		idleDuration := calcDuration(lastEntry.Timestamp, nil)
		b.WriteString(fmt.Sprintf("\n%s: %s\n", msg.FieldStuckDuration, idleDuration))
	}

	buildActivitySection(&b, event.ActivityEntries, msg)

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildStuckWarningContent 构建疑似卡死警告通知内容
// 在 getAgentActivity 二次确认前发送，告知用户 Agent 已长时间无更新
func buildStuckWarningContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionTime + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentCreatedAt, formatStrTime(a.CreatedAt)))
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentDuration, calcDuration(a.CreatedAt, nil)))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentUpdatedAt, formatStrTime(a.UpdatedAt)))
	b.WriteString(fmt.Sprintf("\n%s: %s\n", msg.FieldStuckDuration, formatDuration(event.IdleDuration)))
	b.WriteString(fmt.Sprintf(msg.FieldStuckCheckNotice, formatDuration(event.IdleDuration)) + "\n")

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildStillActiveContent 构建 Agent 活动正常通知内容
// getAgentActivity 二次确认发现 Agent 仍在活动时发送，附活动记录
func buildStillActiveContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	buildActivitySection(&b, event.ActivityEntries, msg)

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildRunningStatusContent 构建 Agent 运行中状态通知内容
func buildRunningStatusContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionTime + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentCreatedAt, formatStrTime(a.CreatedAt)))
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentDuration, calcDuration(a.CreatedAt, nil)))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldAgentUpdatedAt, formatStrTime(a.UpdatedAt)))
	if a.LastUserMessageAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldLastUserAsk, formatStrTime(a.LastUserMessageAt)))
	}

	startTime := a.CreatedAt
	if a.LastUserMessageAt != "" {
		startTime = a.LastUserMessageAt
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldRunningDuration, calcDuration(startTime, nil)))

	// 运行中状态附加子任务进度
	if len(event.Subagents) > 0 {
		b.WriteString(fmt.Sprintf("\n%s: %s\n", msg.SectionSubagents, agentwatcher.FormatSubagentSummary(event.Subagents)))
	}

	buildActivitySection(&b, event.ActivityEntries, msg)

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildActivitySection 向 Builder 写入活动记录摘要（最多 1 条）
func buildActivitySection(b *strings.Builder, entries []agentwatcher.ActivityEntry, msg messages) {
	if len(entries) == 0 {
		return
	}
	b.WriteString("\n" + msg.SectionActivity + "\n")
	limit := 1
	start := 0
	if len(entries) > limit {
		start = len(entries) - limit
	}
	for _, entry := range entries[start:] {
		summary := truncateSummary(entry.Summary, 80)
		if entry.Type != "" && summary != "" {
			b.WriteString(fmt.Sprintf("  • [%s] %s\n", entry.Type, summary))
		} else if summary != "" {
			b.WriteString(fmt.Sprintf("  • %s\n", summary))
		} else if entry.Type != "" {
			b.WriteString(fmt.Sprintf("  • [%s]\n", entry.Type))
		}
	}
}

// truncateSummary 截断过长的摘要文本，多行内容只取首行
func truncateSummary(s string, maxLen int) string {
	if s == "" {
		return ""
	}
	// 多行内容只取首行
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatDuration 将 time.Duration 格式化为 "XdXhXm" / "XmXs" / "Xs" 人类可读格式
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// buildPermissionContent 构建权限请求事件的通知内容
func buildPermissionContent(event agentwatcher.AgentEvent, msg messages) string {
	perm := event.Permission
	if perm == nil {
		return msg.SectionRequest
	}
	var b strings.Builder
	b.WriteString(msg.SectionRequest + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldPermAgent, perm.AgentID))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldPermType, kindToLabel(perm.Request.Kind, msg)))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldProvider, perm.Request.Provider))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldPermTitle, perm.Request.Title))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldPermDesc, perm.Request.Description))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldPermStatus, perm.Status))

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// formatStrTime 解析 RFC3339 时间字符串并格式化为本地时间 "2006-01-02 15:04:05"
// 输入为空时返回 "-"，解析失败返回原始字符串
func formatStrTime(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// formatTime 解析 RFC3339 时间指针并格式化为本地时间 "2006-01-02 15:04:05"
// 输入为 nil 时返回 "-"，解析失败返回原始字符串
func formatTime(ts *string) string {
	if ts == nil {
		return "-"
	}
	return formatStrTime(*ts)
}

// calcDuration 计算两个 RFC3339 时间戳之间的持续时长
// 如果 end 为 nil，使用当前时间作为结束时间
func calcDuration(start string, end *string) string {
	startT, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return "-"
	}
	endT := time.Now()
	if end != nil {
		if t, err := time.Parse(time.RFC3339, *end); err == nil {
			endT = t
		}
	}
	return formatDuration(endT.Sub(startT))
}

// modelLabel 返回人类可读的模型标签，优先使用 "模型 (供应商)" 格式
func modelLabel(agent agentwatcher.AgentStatus) string {
	if agent.Model != "" && agent.Provider != "" {
		return fmt.Sprintf("%s (%s)", agent.Model, agent.Provider)
	}
	if agent.Model != "" {
		return agent.Model
	}
	return agent.Provider
}

// stuckReason 从活动记录中提取卡死原因
// 优先查找 error 类型的条目，否则取最后一条有意义的摘要
func stuckReason(entries []agentwatcher.ActivityEntry) string {
	var lastSummary string
	for i := range entries {
		if entries[i].Summary == "" {
			continue
		}
		lastSummary = entries[i].Summary
		// error 类型或包含 error/retry/timeout 关键词的优先作为原因
		switch entries[i].Type {
		case "error", "fatal":
			return entries[i].Summary
		}
	}
	return lastSummary
}

// lastActivityEntry 返回活动记录中时间戳最新的那条，没有记录时返回 nil
func lastActivityEntry(entries []agentwatcher.ActivityEntry) *agentwatcher.ActivityEntry {
	var latest *agentwatcher.ActivityEntry
	var latestTime time.Time
	for i := range entries {
		if entries[i].Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, entries[i].Timestamp)
		if err != nil {
			continue
		}
		if latest == nil || t.After(latestTime) {
			latest = &entries[i]
			latestTime = t
		}
	}
	return latest
}

// buildSubagentProgressContent 构建子任务进度通知内容
func buildSubagentProgressContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	if len(event.Subagents) > 0 {
		var running, completed []agentwatcher.ProviderSubagentStatus
		for _, sa := range event.Subagents {
			switch sa.Status {
			case "running":
				running = append(running, sa)
			case "completed", "idle", "error":
				completed = append(completed, sa)
			}
		}

		b.WriteString("\n" + msg.SectionSubagents + "\n")

		for _, sa := range running {
			b.WriteString(fmt.Sprintf("  ▶ %s", sa.SubagentID))
			if sa.Title != "" {
				b.WriteString(fmt.Sprintf(" - %s", sa.Title))
			}
			appendStatusLabel(&b, sa.Status)
			b.WriteString("\n")
		}

		for _, sa := range completed {
			marker := "✓"
			if sa.Status == "error" {
				marker = "✗"
			}
			b.WriteString(fmt.Sprintf("  %s %s", marker, sa.SubagentID))
			if sa.Title != "" {
				b.WriteString(fmt.Sprintf(" - %s", sa.Title))
			}
			appendStatusLabel(&b, sa.Status)
			b.WriteString("\n")
		}

		b.WriteString(fmt.Sprintf("  (%s)\n", agentwatcher.FormatSubagentSummary(event.Subagents)))
	}

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildAllSubagentsDoneContent 构建全部 subagent 完成通知内容
func buildAllSubagentsDoneContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionSubagents + "\n")
	for _, sa := range event.Subagents {
		b.WriteString(fmt.Sprintf("  ✓ %s", sa.SubagentID))
		if sa.Title != "" {
			b.WriteString(fmt.Sprintf(" - %s", sa.Title))
		}
		appendStatusLabel(&b, sa.Status)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("  (%d total)\n", len(event.Subagents)))

	// 提醒用户继续主 agent
	b.WriteString(fmt.Sprintf("\n%s\n", msg.AllSubagentsDoneHint))

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildAutoContinueContent 构建自动继续通知内容
func buildAutoContinueContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))
	b.WriteString(fmt.Sprintf("\n%s\n", msg.AutoContinueHint))
	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildStuckContinueContent 构建卡死自动继续通知内容
func buildStuckContinueContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))
	b.WriteString(fmt.Sprintf("\n%s\n", msg.StuckContinueHint))
	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildSubagentSpawnedContent 构建 subagent 首次出现通知内容
func buildSubagentSpawnedContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionSubagents + "\n")
	for _, sa := range event.Subagents {
		b.WriteString(fmt.Sprintf("  ▶ %s", sa.SubagentID))
		if sa.Title != "" {
			b.WriteString(fmt.Sprintf(" - %s", sa.Title))
		}
		appendStatusLabel(&b, sa.Status)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// buildSubagentRunningContent 构建 subagent 持续运行通知内容
func buildSubagentRunningContent(event agentwatcher.AgentEvent, msg messages) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(buildAgentInfoSection(a, msg))

	b.WriteString("\n" + msg.SectionSubagents + "\n")
	for _, sa := range event.Subagents {
		b.WriteString(fmt.Sprintf("  %s %s", subagentRunningMarker(sa.Status), sa.SubagentID))
		if sa.Title != "" {
			b.WriteString(fmt.Sprintf(" - %s", sa.Title))
		}
		appendStatusLabel(&b, sa.Status)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("  (%s)\n", agentwatcher.FormatSubagentSummary(event.Subagents)))

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
}

// appendStatusLabel 仅在状态非空时追加 "[状态]" 标签
func appendStatusLabel(b *strings.Builder, status string) {
	label := agentwatcher.SubagentStatusLabel(status)
	if label != "" {
		b.WriteString(fmt.Sprintf(" [%s]", label))
	}
}

// subagentRunningMarker 返回状态对应的图标标记
func subagentRunningMarker(status string) string {
	switch status {
	case "running":
		return "▶"
	case "completed":
		return "✓"
	case "error":
		return "✗"
	default:
		return "•"
	}
}

// BuildContinuePrompt 返回任务完成后自动继续的提示文本（简短）
func BuildContinuePrompt() string {
	return getMessages(currentLang).ContinuePrompt
}

// BuildStuckContinuePrompt 返回卡死重启时发送给 Agent 的继续提示文本（含上下文说明）
func BuildStuckContinuePrompt() string {
	return getMessages(currentLang).StuckContinuePrompt
}

// kindToLabel 将权限请求的 kind 字符串转换为本地化标签
func kindToLabel(kind string, msg messages) string {
	switch kind {
	case "tool":
		return msg.KindTool
	case "plan":
		return msg.KindPlan
	case "question":
		return msg.KindQuestion
	case "mode":
		return msg.KindMode
	default:
		return kind
	}
}
