package agentwatcher

import (
	"encoding/json"
	"testing"
	"time"
)

type mockNotifier struct {
	events []AgentEvent
}

func (m *mockNotifier) Notify(event AgentEvent) error {
	m.events = append(m.events, event)
	return nil
}

func agentJSON(t *testing.T, id, shortID, title, provider, status, attentionReason string, requiresAttention bool) []byte {
	t.Helper()
	var reason *string
	if attentionReason != "" {
		reason = &attentionReason
	}
	a := AgentStatus{
		ID:                id,
		ShortID:           shortID,
		Title:             title,
		Provider:          provider,
		Status:            status,
		RequiresAttention: requiresAttention,
		AttentionReason:   reason,
	}
	data, _ := json.Marshal(a)
	return data
}

func TestDetectFinishedEvent(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	agent1 := AgentStatus{
		ID:              "agent-1",
		ShortID:         "agent-1",
		Title:           "测试任务",
		Provider:        "codex",
		Status:          "running",
		AttentionReason: nil,
	}

	agent2 := AgentStatus{
		ID:                "agent-1",
		ShortID:           "agent-1",
		Title:             "测试任务",
		Provider:          "codex",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("finished"),
	}

	w.detectAgentChange(agent1)
	w.detectAgentChange(agent2)

	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(notifier.events))
	}
	if notifier.events[0].Type != EventFinished {
		t.Fatalf("expected EventFinished, got %s", notifier.events[0].Type)
	}
	if notifier.events[0].Agent.Title != "测试任务" {
		t.Fatalf("expected title '测试任务', got '%s'", notifier.events[0].Agent.Title)
	}
}

func TestDetectErrorEvent(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	agent1 := AgentStatus{
		ID:              "agent-2",
		ShortID:         "agent-2",
		Title:           "出错任务",
		Provider:        "opencode",
		Status:          "running",
		AttentionReason: nil,
	}

	agent2 := AgentStatus{
		ID:                "agent-2",
		ShortID:           "agent-2",
		Title:             "出错任务",
		Provider:          "opencode",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("error"),
	}

	w.detectAgentChange(agent1)
	w.detectAgentChange(agent2)

	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(notifier.events))
	}
	if notifier.events[0].Type != EventError {
		t.Fatalf("expected EventError, got %s", notifier.events[0].Type)
	}
}

func TestDeduplicateFinishedEvent(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	agent1 := AgentStatus{
		ID:              "agent-3",
		ShortID:         "agent-3",
		Title:           "任务",
		AttentionReason: strPtr("finished"),
	}

	agent2 := AgentStatus{
		ID:              "agent-3",
		ShortID:         "agent-3",
		Title:           "任务",
		AttentionReason: strPtr("finished"),
	}

	w.detectAgentChange(agent1)
	if len(notifier.events) != 0 {
		t.Fatalf("first scan should record state, not trigger event")
	}

	w.detectAgentChange(agent2)
	if len(notifier.events) != 0 {
		t.Fatalf("same state should not trigger again, got %d events", len(notifier.events))
	}
}

func TestDetectNewPermission(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	perm1 := PermissionRequest{
		AgentID: "agent-4",
		Status:  "running",
	}
	perm1.Request.ID = "perm-1"
	perm1.Request.Kind = "tool"
	perm1.Request.Title = "文件写入"
	perm1.Request.Description = "写入 /tmp/test.txt"

	w.detectNewPermission(perm1)

	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(notifier.events))
	}
	if notifier.events[0].Type != EventPermissionRequest {
		t.Fatalf("expected EventPermissionRequest, got %s", notifier.events[0].Type)
	}
}

func TestSkipDuplicatePermission(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	perm1 := PermissionRequest{
		AgentID: "agent-5",
		Status:  "running",
	}
	perm1.Request.ID = "perm-2"
	perm1.Request.Kind = "tool"

	w.detectNewPermission(perm1)
	w.detectNewPermission(perm1)

	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 event (deduplicated), got %d", len(notifier.events))
	}
}

func TestSkipNonRunningPermission(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	perm1 := PermissionRequest{
		AgentID: "agent-6",
		Status:  "idle",
	}
	perm1.Request.ID = "perm-3"
	perm1.Request.Kind = "tool"

	w.detectNewPermission(perm1)

	if len(notifier.events) != 0 {
		t.Fatalf("expected 0 events for non-running permission, got %d", len(notifier.events))
	}
}

func TestSkipArchivedAgent(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	archivedAt := "2026-01-01T00:00:00Z"
	agent1 := AgentStatus{
		ID:              "agent-7",
		ShortID:         "agent-7",
		Status:          "closed",
		AttentionReason: strPtr("finished"),
		ArchivedAt:      &archivedAt,
	}

	w.detectAgentChange(agent1)
	if len(notifier.events) != 0 {
		t.Fatalf("archived agent should not trigger events")
	}
}

// TestDetectSameReasonDifferentTimestamp 测试同一 reason 但不同 timestamp 应触发通知
func TestDetectSameReasonDifferentTimestamp(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	ts1 := "2026-07-01T10:00:00Z"
	ts2 := "2026-07-02T10:00:00Z"

	agent1 := AgentStatus{
		ID:                "agent-8",
		ShortID:           "agent-8",
		Title:             "时间戳测试任务",
		Provider:          "opencode",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("finished"),
		AttentionTimestamp: &ts1,
	}

	agent2 := AgentStatus{
		ID:                "agent-8",
		ShortID:           "agent-8",
		Title:             "时间戳测试任务",
		Provider:          "opencode",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("finished"),
		AttentionTimestamp: &ts2,
	}

	w.detectAgentChange(agent1)
	if len(notifier.events) != 0 {
		t.Fatalf("first scan should record state, not trigger event, got %d events", len(notifier.events))
	}

	w.detectAgentChange(agent2)
	if len(notifier.events) != 1 {
		t.Fatalf("timestamp change should trigger event, got %d events", len(notifier.events))
	}
	if notifier.events[0].Type != EventFinished {
		t.Fatalf("expected EventFinished, got %s", notifier.events[0].Type)
	}
}

// TestDetectReasonChangeDifferentTimestamp 测试不同 reason 且不同 timestamp 应触发通知
func TestDetectReasonChangeDifferentTimestamp(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)

	ts1 := "2026-07-01T10:00:00Z"
	ts2 := "2026-07-02T10:00:00Z"

	agent1 := AgentStatus{
		ID:                "agent-9",
		ShortID:           "agent-9",
		Title:             "混合变更任务",
		Provider:          "opencode",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("finished"),
		AttentionTimestamp: &ts1,
	}

	agent2 := AgentStatus{
		ID:                "agent-9",
		ShortID:           "agent-9",
		Title:             "混合变更任务",
		Provider:          "opencode",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("error"),
		AttentionTimestamp: &ts2,
	}

	w.detectAgentChange(agent1)
	if len(notifier.events) != 0 {
		t.Fatalf("first scan should record state, not trigger event, got %d events", len(notifier.events))
	}

	w.detectAgentChange(agent2)
	if len(notifier.events) != 1 {
		t.Fatalf("reason and timestamp both changed should trigger event, got %d events", len(notifier.events))
	}
	if notifier.events[0].Type != EventError {
		t.Fatalf("expected EventError, got %s", notifier.events[0].Type)
	}
}

func strPtr(s string) *string {
	return &s
}

// mockSysNotifier 模拟系统通知回调
type mockSysNotifier struct {
	disconnected []bool
	daemons      []string
}

func (m *mockSysNotifier) notify(disconnected bool, daemonURL string) {
	m.disconnected = append(m.disconnected, disconnected)
	m.daemons = append(m.daemons, daemonURL)
}

func TestHandleConnStateInitialDisconnect(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)
	w.SetSystemNotifier(sys.notify)

	if w.connState != ConnConnected {
		t.Fatalf("initial state should be connected, got %s", w.connState)
	}

	w.handleConnState(true)

	if w.connState != ConnDisconnected {
		t.Fatalf("after disconnect, state should be disconnected, got %s", w.connState)
	}
	if len(sys.disconnected) != 1 {
		t.Fatalf("expected 1 system notification, got %d", len(sys.disconnected))
	}
}

func TestHandleConnStateReconnect(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)
	w.SetSystemNotifier(sys.notify)

	// 先断开
	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("expected 1 disconnect notification, got %d", len(sys.disconnected))
	}

	// 记录一些历史状态
	w.prevAgents["test"] = &AgentState{
		AttentionReason: strPtr("finished"),
	}
	w.prevPermIDs["test/perm-1"] = true

	// 重连
	w.handleConnState(false)

	if w.connState != ConnConnected {
		t.Fatalf("after reconnect, state should be connected, got %s", w.connState)
	}
	if len(sys.disconnected) != 2 {
		t.Fatalf("expected 2 system notifications (disconnect + reconnect), got %d", len(sys.disconnected))
	}

	// 历史状态应被清空
	if len(w.prevAgents) != 0 {
		t.Fatalf("prevAgents should be cleared after reconnect, got %d entries", len(w.prevAgents))
	}
	if len(w.prevPermIDs) != 0 {
		t.Fatalf("prevPermIDs should be cleared after reconnect, got %d entries", len(w.prevPermIDs))
	}
}

func TestHandleConnStateContinuousDisconnectDoesNotRepeat(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)
	w.SetSystemNotifier(sys.notify)

	// 第一次断开
	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("expected 1 notification after first disconnect, got %d", len(sys.disconnected))
	}

	// 持续断开不应重复通知
	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("continuous disconnect should not repeat notification, got %d", len(sys.disconnected))
	}
}

func TestHandleConnStateContinuousConnectedDoesNotNotify(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := NewWatcher("http://localhost:9999", time.Second, notifier)
	w.SetSystemNotifier(sys.notify)

	// 初始状态已是 connected，再传 false 不应触发通知
	w.handleConnState(false)
	if len(sys.disconnected) != 0 {
		t.Fatalf("continuous connected should not send notifications, got %d", len(sys.disconnected))
	}
}
