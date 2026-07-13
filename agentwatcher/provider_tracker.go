package agentwatcher

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/winezer0/paseo-notifier/logging"
)

// providerSubagentStatusList 状态列表请求载荷
type providerSubagentStatusList struct {
	ParentAgentID string `json:"parentAgentId"`
}

// providerSubagentStatusListResponse 状态列表响应载荷
type providerSubagentStatusListResponse struct {
	Subagents []providerSubagentPayload `json:"subagents"`
}

// ProviderSubagentStatus 服务端推送的单个 provider subagent 状态
type ProviderSubagentStatus struct {
	ParentAgentID string `json:"parentAgentId"`
	SubagentID    string `json:"subagentId"`
	Title         string `json:"title"`
	Provider      string `json:"provider"`
	Model         string `json:"model,omitempty"`
	Status        string `json:"status"` // running / idle / completed / error
}

// ProviderSubagentTracker 通过 WebSocket 推送追踪 provider subagent 完成状态。
// 当某个父 agent 首次出现 subagent、全部完成、或持续运行超时，触发对应回调。
type ProviderSubagentTracker struct {
	mu                sync.Mutex
	subagents         map[string]*ProviderSubagentStatus // key = parentAgentID + "/" + subagentID
	allDoneNotified   map[string]bool                    // 已发送"全部完成"通知的 parent agent
	spawnNotified     map[string]bool                    // 已发送"首个子任务出现"通知的 parent agent
	onAllDone         func(parentAgentID string, subagents []ProviderSubagentStatus)
	onFirstSubagent   func(parentAgentID string, subagent ProviderSubagentStatus)
}

// NewProviderSubagentTracker 创建追踪器
func NewProviderSubagentTracker(onAllDone func(parentAgentID string, subagents []ProviderSubagentStatus), onFirstSubagent func(parentAgentID string, subagent ProviderSubagentStatus)) *ProviderSubagentTracker {
	return &ProviderSubagentTracker{
		subagents:       make(map[string]*ProviderSubagentStatus),
		allDoneNotified: make(map[string]bool),
		spawnNotified:   make(map[string]bool),
		onAllDone:       onAllDone,
		onFirstSubagent: onFirstSubagent,
	}
}

// key 构建存储键
func (t *ProviderSubagentTracker) key(parentID, subID string) string {
	return parentID + "/" + subID
}

// providerSubagentUpdate daemon 推送的 subagent 真实格式
type providerSubagentUpdate struct {
	Kind     string                   `json:"kind"`     // "upsert" | "timeline"
	Subagent *providerSubagentPayload `json:"subagent"` // 仅 upsert 有值
}

// providerSubagentPayload subagent 内层描述符
type providerSubagentPayload struct {
	ID            string `json:"id"`
	ParentAgentID string `json:"parentAgentId"`
	Title         string `json:"title"`
	Provider      string `json:"provider"`
	Model         string `json:"model,omitempty"`
	Status        string `json:"status"`
}

// HandleSubagentUpdate 处理 WebSocket 推送的 "agent.provider_subagents.update" 消息
func (t *ProviderSubagentTracker) HandleSubagentUpdate(payload json.RawMessage) {
	// 尝试按真实格式解析
	var raw providerSubagentUpdate
	if err := json.Unmarshal(payload, &raw); err != nil || raw.Subagent == nil {
		return
	}
	sa := raw.Subagent
	if sa.ParentAgentID == "" || sa.ID == "" {
		return
	}

	update := ProviderSubagentStatus{
		ParentAgentID: sa.ParentAgentID,
		SubagentID:    sa.ID,
		Title:         sa.Title,
		Provider:      sa.Provider,
		Model:         sa.Model,
		Status:        sa.Status,
	}

	t.mu.Lock()

	k := t.key(update.ParentAgentID, update.SubagentID)
	existing, exists := t.subagents[k]

	// 收集需要在锁外执行的回调（避免持锁做 HTTP / 通知等耗时操作）
	var spawnCb func()
	var allDoneCb func()

	if !exists {
		delete(t.allDoneNotified, update.ParentAgentID)
		updateCopy := update
		t.subagents[k] = &updateCopy
		logging.Debugf("provider_subagent new parent=%s sub=%s status=%s title=%s",
			update.ParentAgentID, update.SubagentID, update.Status, update.Title)

		if !t.spawnNotified[update.ParentAgentID] {
			t.spawnNotified[update.ParentAgentID] = true
			if t.onFirstSubagent != nil {
				cb := t.onFirstSubagent
				pid := update.ParentAgentID
				sa := updateCopy
				spawnCb = func() { cb(pid, sa) }
			}
		}
	} else if existing.Status != update.Status {
		if existing.Status == "completed" && update.Status == "running" {
			delete(t.allDoneNotified, update.ParentAgentID)
		}
		existing.Status = update.Status
		existing.Title = update.Title
		existing.Model = update.Model
		existing.Provider = update.Provider
		logging.Debugf("provider_subagent status parent=%s sub=%s %s→%s",
			update.ParentAgentID, update.SubagentID, existing.Status, update.Status)
	}

	// 检查是否全部完成（仍在锁内判断，但回调在锁外执行）
	if notified, subs := t.checkAllDoneLocked(update.ParentAgentID); notified {
		if t.onAllDone != nil {
			cb := t.onAllDone
			pid := update.ParentAgentID
			allDoneCb = func() { cb(pid, subs) }
		}
	}

	t.mu.Unlock()

	// 锁外执行回调
	if spawnCb != nil {
		spawnCb()
	}
	if allDoneCb != nil {
		allDoneCb()
	}
}

// HandleSubagentList 处理 "agent.provider_subagents.list.response" 消息，
// 用于重连后基线同步
func (t *ProviderSubagentTracker) HandleSubagentList(payload json.RawMessage) {
	var resp providerSubagentStatusListResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		logging.Debugf("provider_subagent parse list response failed: %v", err)
		return
	}

	t.mu.Lock()
	for _, sa := range resp.Subagents {
		if sa.ParentAgentID == "" || sa.ID == "" {
			continue
		}
		k := t.key(sa.ParentAgentID, sa.ID)
		existing, exists := t.subagents[k]
		if exists {
			// 仅在 list 数据有值时更新，避免空值覆盖 WS 推送的实时状态
			if sa.Status != "" {
				existing.Status = sa.Status
			}
			if sa.Title != "" {
				existing.Title = sa.Title
			}
			if sa.Model != "" {
				existing.Model = sa.Model
			}
			if sa.Provider != "" {
				existing.Provider = sa.Provider
			}
		} else {
			t.subagents[k] = &ProviderSubagentStatus{
				ParentAgentID: sa.ParentAgentID,
				SubagentID:    sa.ID,
				Title:         sa.Title,
				Provider:      sa.Provider,
				Model:         sa.Model,
				Status:        sa.Status,
			}
		}
	}

	// 收集需要通知的 completed parents（锁内判断，锁外回调）
	type doneEntry struct {
		parentID string
		subs     []ProviderSubagentStatus
	}
	var doneEntries []doneEntry
	for _, pid := range t.collectParents() {
		if notified, subs := t.checkAllDoneLocked(pid); notified {
			doneEntries = append(doneEntries, doneEntry{pid, subs})
		}
	}
	t.mu.Unlock()

	for _, e := range doneEntries {
		if t.onAllDone != nil {
			t.onAllDone(e.parentID, e.subs)
		}
	}
}

// collectParents 收集所有已知的 parent agent ID（需持有锁）
func (t *ProviderSubagentTracker) collectParents() []string {
	seen := make(map[string]bool)
	for _, sa := range t.subagents {
		seen[sa.ParentAgentID] = true
	}
	result := make([]string, 0, len(seen))
	for pid := range seen {
		result = append(result, pid)
	}
	return result
}

// checkAllDoneLocked 检查指定父 agent 的所有 subagent 是否全部完成（需持有锁）。
// 返回 (是否应通知, 该 parent 的全部 subagent 快照)。
// 不调用回调——由调用方在锁外执行。
func (t *ProviderSubagentTracker) checkAllDoneLocked(parentID string) (bool, []ProviderSubagentStatus) {
	if t.allDoneNotified[parentID] {
		return false, nil
	}

	hasAny := false
	for _, sa := range t.subagents {
		if sa.ParentAgentID != parentID {
			continue
		}
		hasAny = true
		if sa.Status == "running" {
			return false, nil
		}
	}

	if !hasAny {
		return false, nil
	}

	t.allDoneNotified[parentID] = true

	var all []ProviderSubagentStatus
	for _, sa := range t.subagents {
		if sa.ParentAgentID == parentID {
			all = append(all, *sa)
		}
	}

	logging.Infof("provider_subagent all done parent=%s count=%d", parentID, len(all))
	return true, all
}

// GetByParent 返回指定父 agent 的所有 subagent（用于心跳通知）
func (t *ProviderSubagentTracker) GetByParent(parentID string) []ProviderSubagentStatus {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []ProviderSubagentStatus
	for _, sa := range t.subagents {
		if sa.ParentAgentID == parentID {
			result = append(result, *sa)
		}
	}
	return result
}

// GetTrackedParents 返回所有追踪了子任务的 parent agent ID 列表
func (t *ProviderSubagentTracker) GetTrackedParents() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.collectParents()
}

// HasRunningSubagents 检查指定父 agent 是否有仍在运行的 subagent
func (t *ProviderSubagentTracker) HasRunningSubagents(parentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, sa := range t.subagents {
		if sa.ParentAgentID == parentID && sa.Status == "running" {
			return true
		}
	}
	return false
}

// Reset 清空子任务数据（重连时调用），但保留通知状态避免重复通知
func (t *ProviderSubagentTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subagents = make(map[string]*ProviderSubagentStatus)
	// allDoneNotified / spawnNotified 不清空——重连后不应重复通知已完成的 subagent
}

// BuildListRequest 构建请求 subagent 列表的消息
func BuildListRequest(requestID string, parentAgentID string) WSMessage {
	payload, _ := json.Marshal(providerSubagentStatusList{ParentAgentID: parentAgentID})
	return WSMessage{
		Type:      "agent.provider_subagents.list.request",
		RequestID: requestID,
		Payload:   payload,
	}
}

// BuildListResponseType 返回列表响应的消息类型
func BuildListResponseType() string {
	return "agent.provider_subagents.list.response"
}

// BuildUpdateType 返回实时更新的消息类型
func BuildUpdateType() string {
	return "agent.provider_subagents.update"
}

// subagentStatusSummary 计算子任务汇总（running/completed/error/idle 数量）
func subagentStatusSummary(subagents []ProviderSubagentStatus) (running, completed, errored, idle int) {
	for _, sa := range subagents {
		switch sa.Status {
		case "running":
			running++
		case "completed":
			completed++
		case "error":
			errored++
		case "idle":
			idle++
		}
	}
	return
}

// SubagentStatusLabel 返回状态标签，空状态返回空字符串
func SubagentStatusLabel(status string) string {
	switch status {
	case "running":
		return "运行中"
	case "completed":
		return "已完成"
	case "error":
		return "出错"
	case "idle":
		return "空闲"
	case "":
		return ""
	default:
		return status
	}
}

// FormatSubagentSummary 格式化子任务通知汇总文本
func FormatSubagentSummary(subagents []ProviderSubagentStatus) string {
	running, completed, errored, idle := subagentStatusSummary(subagents)
	var parts []string
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if completed > 0 {
		parts = append(parts, fmt.Sprintf("%d completed", completed))
	}
	if errored > 0 {
		parts = append(parts, fmt.Sprintf("%d error", errored))
	}
	if idle > 0 {
		parts = append(parts, fmt.Sprintf("%d idle", idle))
	}
	if len(parts) == 0 {
		return "0 total"
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return fmt.Sprintf("%s (%d total)", result, len(subagents))
}
