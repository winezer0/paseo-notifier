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
)

// AgentEvent 表示 Agent 状态变更事件
type AgentEvent struct {
	Type             EventType
	Agent            AgentStatus
	Timestamp        time.Time
	Permission       *PermissionRequest
	ActivityEntries  []ActivityEntry
	IdleDuration     time.Duration // 卡死相关事件的静止时长（UpdatedAt 无变化时间）
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

// SystemNotifyFunc 系统事件通知回调（连接断开/重连）
// disconnected=true 表示断连，false 表示重连
type SystemNotifyFunc func(disconnected bool, daemonURL string)

// AgentState 内部追踪的上次 Agent 状态快照
type AgentState struct {
	AttentionReason    *string
	AttentionTimestamp *string
	LastUpdatedAt      string // 上次见到的 UpdatedAt 值，用于卡死检测
	StuckSince         string // 首次检测到 UpdatedAt 无变化的时间（RFC3339），空串表示未卡死
	StuckNotified      bool   // 是否已发送确认卡死通知
	StuckActionTaken   bool   // 是否已执行自动重启操作
	RetryCount         int    // 重启重试次数，达到 maxRetries 后执行复活
	StuckWarningSent   bool   // 是否已发送疑似卡死警告
	StillActiveNotified bool  // 是否已发送活动正常通知
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