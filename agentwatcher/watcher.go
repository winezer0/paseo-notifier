package agentwatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
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
)

// AgentEvent 表示 Agent 状态变更事件
type AgentEvent struct {
	Type             EventType
	Agent            AgentStatus
	Timestamp        time.Time
	Permission       *PermissionRequest
	ActivityEntries  []ActivityEntry
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

// SystemNotifyFunc 系统事件通知回调（连接断开/重连）
// disconnected=true 表示断连，false 表示重连
type SystemNotifyFunc func(disconnected bool, daemonURL string)

// Watcher 通过 MCP API 轮询监控 Agent 状态
type Watcher struct {
	daemonURL    string
	interval     time.Duration
	stuckTimeout time.Duration
	notifier     Notifier
	sysNotifyFn  SystemNotifyFunc
	connState    ConnState
	prevAgents   map[string]*AgentState
	prevPermIDs  map[string]bool
	httpClient   *http.Client
	done         chan struct{}
	reqID        int
	ctx          context.Context
	cancel       context.CancelFunc
}

// AgentState 内部追踪的上次 Agent 状态快照
type AgentState struct {
	AttentionReason    *string
	AttentionTimestamp *string
	LastUpdatedAt      string // 上次见到的 UpdatedAt 值，用于卡死检测
	StuckSince         string // 首次检测到 UpdatedAt 无变化的时间（RFC3339），空串表示未卡死
	StuckNotified      bool   // 是否已发送卡死通知，避免重复通知
}

// NewWatcher 创建新的 Agent 状态监控器
func NewWatcher(daemonURL string, interval time.Duration, notifier Notifier) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watcher{
		daemonURL:    daemonURL,
		interval:     interval,
		stuckTimeout: 3 * time.Minute,
		notifier:     notifier,
		connState:    ConnConnected,
		prevAgents:   make(map[string]*AgentState),
		prevPermIDs:  make(map[string]bool),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 5,
				DisableCompression:  false,
			},
		},
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
		reqID:  0,
	}
}

// SetStuckTimeout 设置卡死检测的超时时间
func (w *Watcher) SetStuckTimeout(timeout time.Duration) {
	if timeout > 0 {
		w.stuckTimeout = timeout
	}
}

// SetSystemNotifier 设置系统事件通知回调
func (w *Watcher) SetSystemNotifier(fn SystemNotifyFunc) {
	w.sysNotifyFn = fn
}

func (w *Watcher) nextID() int {
	w.reqID++
	return w.reqID
}

// Start 开始轮询监控
func (w *Watcher) Start() {
	logging.Infof("agent watcher started daemon=%s interval=%s", w.daemonURL, w.interval)

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		w.pollOnce()

		for {
			select {
			case <-ticker.C:
				w.pollOnce()
			case <-w.done:
				logging.Info("agent watcher stopped")
				return
			}
		}
	}()
}

// Stop 停止轮询监控
func (w *Watcher) Stop() {
	w.cancel()
	close(w.done)
}

func (w *Watcher) pollOnce() {
	// 一次探活决定全局连接状态
	agents, agentsErr := w.fetchAgents()

	disconnected := agentsErr != nil

	// 如果 agents 请求成功，再请求 permissions
	var perms []PermissionRequest
	if !disconnected {
		var permsErr error
		perms, permsErr = w.fetchPendingPermissions()
		if permsErr != nil {
			logging.Warnf("fetch permissions failed: %v", permsErr)
		}
	}

	w.handleConnState(disconnected)
	if disconnected {
		return
	}

	for _, agent := range agents {
		w.detectAgentChange(agent)
	}
	w.detectStuckAgents(agents)
	for _, perm := range perms {
		w.detectNewPermission(perm)
	}
}

// handleConnState 处理连接状态转换
func (w *Watcher) handleConnState(disconnected bool) {
	switch {
	case disconnected && w.connState == ConnConnected:
		// ① 从连接 → 断开
		w.connState = ConnDisconnected
		w.sendDisconnectedNotify()

	case !disconnected && w.connState == ConnDisconnected:
		// ② 从断开 → 重连
		w.connState = ConnConnected
		w.prevAgents = make(map[string]*AgentState)
		w.prevPermIDs = make(map[string]bool)
		w.sendReconnectedNotify()

	case disconnected && w.connState == ConnDisconnected:
		// ③ 持续断开，不做操作

	case !disconnected && w.connState == ConnConnected:
		// ④ 持续连接，不做操作
	}
}

func (w *Watcher) sendDisconnectedNotify() {
	logging.Warn("mcp daemon disconnected, agent notifications paused")
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(true, w.daemonURL)
	}
}

func (w *Watcher) sendReconnectedNotify() {
	logging.Info("mcp daemon reconnected, agent notifications resumed")
	if w.sysNotifyFn != nil {
		w.sysNotifyFn(false, w.daemonURL)
	}
}

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
func (w *Watcher) detectStuckAgents(agents []AgentStatus) {
	now := time.Now()
	for _, agent := range agents {
		if agent.ArchivedAt != nil {
			continue
		}
		// 已经 finished/error 的不需要检查
		if agent.AttentionReason != nil {
			continue
		}

		prev, exists := w.prevAgents[agent.ID]
		if !exists {
			continue
		}

		// UpdatedAt 为空或已变化 → 重置卡死状态，更新追踪值
		if agent.UpdatedAt == "" {
			prev.StuckSince = ""
			prev.StuckNotified = false
			continue
		}
		if agent.UpdatedAt != prev.LastUpdatedAt {
			prev.LastUpdatedAt = agent.UpdatedAt
			prev.StuckSince = ""
			prev.StuckNotified = false
			continue
		}

		// UpdatedAt 无变化 — 可能卡死
		if prev.StuckSince == "" {
			prev.StuckSince = now.Format(time.RFC3339)
			continue
		}

		if prev.StuckNotified {
			continue
		}

		stuckSince, err := time.Parse(time.RFC3339, prev.StuckSince)
		if err != nil {
			prev.StuckSince = ""
			continue
		}

		if now.Sub(stuckSince) < w.stuckTimeout {
			continue
		}

		// 超时阈值达到，查询活动时间线做二次确认
		entries := w.getAgentActivity(agent.ID)
		lastActivityTime := w.lastActivityTime(entries)

		if lastActivityTime != nil {
			idleDuration := now.Sub(*lastActivityTime)
			if idleDuration < w.stuckTimeout {
				// 最后活动时间在超时阈值内，说明 Agent 还在工作
				logging.Debugf("agent still active agentId=%s lastActivity=%s idle=%v", agent.ShortID, lastActivityTime.Format(time.RFC3339), idleDuration)
				prev.StuckSince = ""
				prev.StuckNotified = false
				continue
			}
			logging.Debugf("agent idle too long agentId=%s idle=%v stuckTimeout=%v", agent.ShortID, idleDuration, w.stuckTimeout)
		} else {
			logging.Debugf("no activity entries found agentId=%s, treating as stuck", agent.ShortID)
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
	}
}

// getAgentActivity 调用 get_agent_activity MCP 工具，返回 Agent 的活动记录列表
// 调用失败时返回 nil
func (w *Watcher) getAgentActivity(agentID string) []ActivityEntry {
	resp, err := w.callMCP("get_agent_activity", map[string]interface{}{
		"agentId": agentID,
	})
	if err != nil {
		logging.Warnf("get_agent_activity failed agentId=%s err=%v", agentID, err)
		return nil
	}

	var result agentActivityResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		logging.Warnf("parse agent activity failed agentId=%s err=%v", agentID, err)
		return nil
	}

	if result.Result.StructuredContent == nil {
		return nil
	}

	return result.Result.StructuredContent.Entries
}

// lastActivityTime 返回活动记录中最后一条的时间戳，没有记录时返回 nil
func (w *Watcher) lastActivityTime(entries []ActivityEntry) *time.Time {
	var latest *time.Time
	for _, entry := range entries {
		if entry.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		if latest == nil || t.After(*latest) {
			latest = &t
		}
	}
	return latest
}

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

func (w *Watcher) fetchAgents() ([]AgentStatus, error) {
	resp, err := w.callMCP("list_agents", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("mcp call failed: %w", err)
	}

	var result listAgentsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse agents response failed: %w", err)
	}

	return result.Result.StructuredContent.Agents, nil
}

func (w *Watcher) fetchPendingPermissions() ([]PermissionRequest, error) {
	resp, err := w.callMCP("list_pending_permissions", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("mcp call failed: %w", err)
	}

	var result listPermissionsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse permissions response failed: %w", err)
	}

	return result.Result.StructuredContent.Permissions, nil
}

func (w *Watcher) callMCP(method string, params interface{}) ([]byte, error) {
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      w.nextID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      method,
			"arguments": params,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(w.ctx, "POST", w.daemonURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSSEJSON(respBody)
}

// parseSSEJSON 从 SSE 响应中提取 data: 行的 JSON 内容
func parseSSEJSON(data []byte) ([]byte, error) {
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data: ")) {
			return bytes.TrimPrefix(trimmed, []byte("data: ")), nil
		}
	}
	return nil, fmt.Errorf("no data line found in SSE response: %s", string(data))
}

// ptrTimeEqual 比较两个 *string 时间戳是否相等
func ptrTimeEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
