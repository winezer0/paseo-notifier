package message

import (
	"os"
	"strings"
)

// Lang 语言类型
type Lang string

const (
	LangAuto Lang = "auto"
	LangZh   Lang = "zh"
	LangEn   Lang = "en"
)

// messages 文本集合（中英双语）
type messages struct {
	// 事件标题
	SubjectFinished   string
	SubjectError      string
	SubjectPermission string
	SubjectStartup    string
	SubjectDisconnect string
	SubjectReconnect  string

	// 内容区块标题
	SectionAgent   string
	SectionTime    string
	SectionLabels  string
	SectionRequest string

	// 字段标签
	FieldTitle      string
	FieldAgent      string
	FieldModel      string
	FieldThinking   string
	FieldDirectory  string
	FieldCreated    string
	FieldLastUser   string
	FieldCompleted  string
	FieldFailedAt   string
	FieldDuration   string
	FieldUpdated    string
	FieldPermAgent  string
	FieldPermType   string
	FieldPermTitle  string
	FieldPermDesc   string
	FieldPermStatus string
	FieldProvider   string
	FieldSeparator  string
	FieldFrom       string
	FieldDaemon     string
	FieldTime       string
	FieldStatus     string

	// 启动通知
	StartupContent string

	// 断连/重连通知
	DisconnectContent string
	ReconnectContent  string

	// kind 标签
	KindTool     string
	KindPlan     string
	KindQuestion string
	KindMode     string
}

var msgZh = messages{
	SubjectFinished:   ":white_check_mark: Agent 任务完成",
	SubjectError:      ":x: Agent 任务失败",
	SubjectPermission: ":warning: Agent 需要用户确认",
	SubjectStartup:    ":bell: %s 已启动",
	SubjectDisconnect: "[已断开] MCP 守护进程连接断开",
	SubjectReconnect:  "[已重连] MCP 守护进程连接恢复",

	SectionAgent:   "--- Agent 信息 ---",
	SectionTime:    "--- 时间信息 ---",
	SectionLabels:  "--- 标签 ---",
	SectionRequest: "--- 请求信息 ---",

	FieldTitle:      "标题",
	FieldAgent:      "Agent",
	FieldModel:      "模型",
	FieldThinking:   "思考模式",
	FieldDirectory:  "目录",
	FieldCreated:    "创建时间",
	FieldLastUser:   "最后用户消息",
	FieldCompleted:  "完成时间",
	FieldFailedAt:   "失败时间",
	FieldDuration:   "运行时长",
	FieldUpdated:    "最后更新",
	FieldPermAgent:  "Agent",
	FieldPermType:   "类型",
	FieldPermTitle:  "标题",
	FieldPermDesc:   "描述",
	FieldPermStatus: "状态",
	FieldProvider:   "供应商",
	FieldSeparator:  "--------------------------",
	FieldFrom:       "来自",
	FieldDaemon:     "守护进程",
	FieldTime:       "时间",
	FieldStatus:     "状态",

	StartupContent:    "通知进程已启动，正在监控 MCP 守护进程连接...",
	DisconnectContent: "状态：已断开，Agent 通知已暂停，重连后将自动恢复",
	ReconnectContent:  "状态：已重连，Agent 通知已恢复",

	KindTool:     "工具调用",
	KindPlan:     "执行计划",
	KindQuestion: "提问",
	KindMode:     "模式切换",
}

var msgEn = messages{
	SubjectFinished:   ":white_check_mark: Agent task completed",
	SubjectError:      ":x: Agent task failed",
	SubjectPermission: ":warning: Agent requires user confirmation",
	SubjectStartup:    ":bell: %s started",
	SubjectDisconnect: "[DISCONNECTED] MCP daemon disconnected",
	SubjectReconnect:  "[RECONNECTED] MCP daemon reconnected",

	SectionAgent:   "--- Agent Info ---",
	SectionTime:    "--- Time Info ---",
	SectionLabels:  "--- Labels ---",
	SectionRequest: "--- Request Info ---",

	FieldTitle:      "Title",
	FieldAgent:      "Agent",
	FieldModel:      "Model",
	FieldThinking:   "Thinking",
	FieldDirectory:  "Directory",
	FieldCreated:    "Created",
	FieldLastUser:   "Last user message",
	FieldCompleted:  "Completed",
	FieldFailedAt:   "Failed at",
	FieldDuration:   "Duration",
	FieldUpdated:    "Last updated",
	FieldPermAgent:  "Agent",
	FieldPermType:   "Type",
	FieldPermTitle:  "Title",
	FieldPermDesc:   "Description",
	FieldPermStatus: "Status",
	FieldProvider:   "Provider",
	FieldSeparator:  "--------------------------",
	FieldFrom:       "from",
	FieldDaemon:     "Daemon",
	FieldTime:       "Time",
	FieldStatus:     "Status",

	StartupContent:    "Notification process started, monitoring MCP daemon connection...",
	DisconnectContent: "Status: Disconnected, agent notifications paused, will resume on reconnect",
	ReconnectContent:  "Status: Reconnected, agent notifications resumed",

	KindTool:     "tool call",
	KindPlan:     "execute plan",
	KindQuestion: "question",
	KindMode:     "mode switch",
}

// getMessages 根据语言返回对应的文本集合
func getMessages(lang Lang) messages {
	switch lang {
	case LangZh:
		return msgZh
	case LangEn:
		return msgEn
	default:
		return msgEn
	}
}

// detectLang 自动检测系统语言，检查环境变量和平台特定 API
func detectLang() Lang {
	// 1. 优先检查 LANG / LC_ALL / LC_MESSAGES 环境变量（Linux/macOS/通用）
	for _, env := range []string{"LANG", "LC_ALL", "LC_MESSAGES"} {
		if v := os.Getenv(env); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "zh") {
				return LangZh
			}
		}
	}

	// 2. 平台特定检测
	if isSystemChinese() {
		return LangZh
	}

	return LangEn
}

// ResolveLang 解析配置的 language 值
// auto → 自动检测；zh/en → 直接使用
func ResolveLang(configured string) Lang {
	lang := Lang(configured)
	if lang == "" || lang == LangAuto {
		return detectLang()
	}
	return lang
}
