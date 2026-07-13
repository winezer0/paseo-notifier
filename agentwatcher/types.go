package agentwatcher

import (
	"context"
	"time"
)

// AgentStatus 对应 list_agents 返回的单个 Agent 状态
type AgentStatus struct {
	ID                  string            `json:"id"`
	ShortID             string            `json:"shortId"`
	Title               string            `json:"title"`
	Provider            string            `json:"provider"`
	Model               string            `json:"model"`
	ThinkingOptionID    *string           `json:"thinkingOptionId"`
	EffectiveThinkingID *string           `json:"effectiveThinkingOptionId"`
	Status              string            `json:"status"`
	CWD                 string            `json:"cwd"`
	CreatedAt           string            `json:"createdAt"`
	UpdatedAt           string            `json:"updatedAt"`
	LastUserMessageAt   string            `json:"lastUserMessageAt"`
	RequiresAttention   bool              `json:"requiresAttention"`
	AttentionReason     *string           `json:"attentionReason"`
	AttentionTimestamp  *string           `json:"attentionTimestamp"`
	ArchivedAt          *string           `json:"archivedAt"`
	Labels              map[string]string `json:"labels"`
}

// PermissionRequest 对应 list_pending_permissions 的权限请求
type PermissionRequest struct {
	AgentID string `json:"agentId"`
	Status  string `json:"status"`
	Request struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"request"`
}

// ActivityEntry get_agent_activity 返回的单条活动记录
type ActivityEntry struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Summary   string `json:"summary"`
}

// EventType 表示事件类型
type EventType string

const (
	EventFinished          EventType = "finished"
	EventError             EventType = "error"
	EventPermissionRequest EventType = "permission_requested"
	EventStuck             EventType = "stuck"
	EventStuckWarning      EventType = "stuck_warning"
	EventStillActive       EventType = "still_active"
	EventRunningStatus     EventType = "running_status"
	EventSubagentProgress  EventType = "subagent_progress"
	EventAllSubagentsDone  EventType = "all_subagents_done" // 全部 subagent 已完成
	EventSubagentSpawned   EventType = "subagent_spawned"   // 首次检测到 subagent
	EventSubagentRunning   EventType = "subagent_running"   // subagent 持续运行中
	EventAutoContinue      EventType = "auto_continue"
	EventStuckContinue     EventType = "stuck_continue" // 卡死后自动发送继续提示
	EventStartup           EventType = "startup"        // 启动通知
	EventDisconnect        EventType = "disconnect"     // 断连通知
	EventReconnect         EventType = "reconnect"      // 重连通知
)

// AgentEvent 表示 Agent 状态变更事件
type AgentEvent struct {
	Type            EventType
	Agent           AgentStatus
	Timestamp       time.Time
	Permission      *PermissionRequest
	ActivityEntries []ActivityEntry
	IdleDuration    time.Duration            // 卡死相关事件的静止时长
	Subagents       []ProviderSubagentStatus // 子任务列表
}

// Notifier 通知器接口
type Notifier interface {
	Notify(ctx context.Context, event AgentEvent) error
}

// ConnState 连接状态
type ConnState string

const (
	ConnConnected    ConnState = "connected"
	ConnDisconnected ConnState = "disconnected"
)

// SystemNotifyFunc 系统事件通知回调
type SystemNotifyFunc func(disconnected bool, daemonURL string)

// AgentState 内部追踪的上次 Agent 状态快照
type AgentState struct {
	AttentionReason     *string
	AttentionTimestamp  *string
	LastUpdatedAt       string
	StuckSince          string
	StuckNotified       bool
	StuckActionTaken    bool
	RetryCount          int
	StuckWarningSent    bool
	StillActiveNotified bool
	StuckChecking       bool
	LastRunningNotify   *time.Time
}

// listAgentsResponse MCP list_agents 响应结构
type listAgentsResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent struct {
			Agents []AgentStatus `json:"agents"`
		} `json:"structuredContent"`
	} `json:"result"`
}

// listPermissionsResponse MCP list_pending_permissions 响应结构
type listPermissionsResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent struct {
			Permissions []PermissionRequest `json:"permissions"`
		} `json:"structuredContent"`
	} `json:"result"`
}

// agentActivityResponse MCP get_agent_activity 响应结构
type agentActivityResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent *struct {
			Entries []ActivityEntry `json:"entries"`
		} `json:"structuredContent"`
	} `json:"result"`
}

// mcpRequest MCP JSON-RPC 请求
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}
