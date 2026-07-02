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
	}
	return
}

// buildFinishedContent 构建 Agent 任务完成通知内容
func buildFinishedContent(event agentwatcher.AgentEvent, msg messages) string {
	return buildAgentTimeContent(event, msg, msg.FieldCompleted)
}

// buildErrorContent 构建 Agent 任务失败通知内容
func buildErrorContent(event agentwatcher.AgentEvent, msg messages) string {
	return buildAgentTimeContent(event, msg, msg.FieldFailedAt)
}

// buildAgentTimeContent 构建 Agent 完成或失败通知的时间内容
// timeLabel 为时间戳字段的标签（"完成时间" 或 "失败时间"）
func buildAgentTimeContent(event agentwatcher.AgentEvent, msg messages, timeLabel string) string {
	a := event.Agent
	var b strings.Builder
	b.WriteString(msg.SectionAgent + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldTitle, a.Title))
	b.WriteString(fmt.Sprintf("%s: %s (%s)\n", msg.FieldAgent, a.ShortID, a.ID))
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldModel, modelLabel(a)))
	if a.ThinkingOptionID != nil && *a.ThinkingOptionID != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldThinking, *a.ThinkingOptionID))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldDirectory, a.CWD))

	b.WriteString("\n" + msg.SectionTime + "\n")
	b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldCreated, formatStrTime(a.CreatedAt)))
	if a.LastUserMessageAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldLastUser, formatStrTime(a.LastUserMessageAt)))
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", timeLabel, formatTime(a.AttentionTimestamp)))
	startTime := a.CreatedAt
	if a.LastUserMessageAt != "" {
		startTime = a.LastUserMessageAt
	}
	if a.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldDuration, calcDuration(startTime, a.AttentionTimestamp)))
	}
	if a.UpdatedAt != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", msg.FieldUpdated, formatStrTime(a.UpdatedAt)))
	}

	if len(a.Labels) > 0 {
		b.WriteString("\n" + msg.SectionLabels + "\n")
		for k, v := range a.Labels {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	b.WriteString(fmt.Sprintf("\n%s\n%s %s", msg.FieldSeparator, msg.FieldFrom, config.AppName))
	return b.String()
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
// 如果 end 为 nil，使用当前时间作为结束时间，返回 "XdXhXm" 或 "Xm" 格式
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
	d := endT.Sub(startT)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
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
