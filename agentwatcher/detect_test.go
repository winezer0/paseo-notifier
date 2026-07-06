package agentwatcher

import (
	"testing"
	"time"
)

// ─────────────── detectAgentChange tests ───────────────

func TestDetectFinishedEvent(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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
	w := testWatcher(notifier)

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

func TestDetectSameReasonDifferentTimestamp(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

	ts1 := "2026-07-01T10:00:00Z"
	ts2 := "2026-07-02T10:00:00Z"

	agent1 := AgentStatus{
		ID:                 "agent-8",
		ShortID:            "agent-8",
		Title:              "时间戳测试任务",
		Provider:           "opencode",
		Status:             "idle",
		RequiresAttention:  true,
		AttentionReason:    strPtr("finished"),
		AttentionTimestamp: &ts1,
	}

	agent2 := AgentStatus{
		ID:                 "agent-8",
		ShortID:            "agent-8",
		Title:              "时间戳测试任务",
		Provider:           "opencode",
		Status:             "idle",
		RequiresAttention:  true,
		AttentionReason:    strPtr("finished"),
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

func TestDetectReasonChangeDifferentTimestamp(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

	ts1 := "2026-07-01T10:00:00Z"
	ts2 := "2026-07-02T10:00:00Z"

	agent1 := AgentStatus{
		ID:                 "agent-9",
		ShortID:            "agent-9",
		Title:              "混合变更任务",
		Provider:           "opencode",
		Status:             "idle",
		RequiresAttention:  true,
		AttentionReason:    strPtr("finished"),
		AttentionTimestamp: &ts1,
	}

	agent2 := AgentStatus{
		ID:                 "agent-9",
		ShortID:            "agent-9",
		Title:              "混合变更任务",
		Provider:           "opencode",
		Status:             "idle",
		RequiresAttention:  true,
		AttentionReason:    strPtr("error"),
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

// ─────────────── detectStuckAgents tests ───────────────

func TestSkipArchivedAgentInStuck(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)
	w.SetStuckDetectTimeout(1)

	archivedAt := "2026-01-01T00:00:00Z"
	agent := AgentStatus{
		ID:         "agent-stuck-1",
		ShortID:    "agent-stuck-1",
		Title:      "stuck test archived",
		Provider:   "codex",
		Status:     "running",
		UpdatedAt:  "2026-07-01T10:00:00Z",
		ArchivedAt: &archivedAt,
	}

	w.prevAgents[agent.ID] = &AgentState{
		LastUpdatedAt: agent.UpdatedAt,
		StuckSince:    time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	}

	w.detectStuckAgents([]AgentStatus{agent})

	if len(notifier.events) != 0 {
		t.Fatalf("archived agent should not trigger stuck event, got %d", len(notifier.events))
	}
}

// TestSkipAttentionReasonAgentInStuck 已 finished/error 的 Agent 应被卡死检测跳过
func TestSkipAttentionReasonAgentInStuck(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)
	w.SetStuckDetectTimeout(1)

	agent := AgentStatus{
		ID:              "agent-stuck-2",
		ShortID:         "agent-stuck-2",
		Title:           "finished task",
		Provider:        "codex",
		Status:          "idle",
		UpdatedAt:       "2026-07-01T10:00:00Z",
		AttentionReason: strPtr("finished"),
	}

	w.prevAgents[agent.ID] = &AgentState{
		LastUpdatedAt: agent.UpdatedAt,
		StuckSince:    time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	}

	w.detectStuckAgents([]AgentStatus{agent})

	if len(notifier.events) != 0 {
		t.Fatalf("finished agent should not trigger stuck event, got %d", len(notifier.events))
	}
}

func TestStuckResetOnUpdatedAtChange(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

	agent := AgentStatus{
		ID:        "agent-stuck-3",
		ShortID:   "agent-stuck-3",
		Title:     "running task",
		Provider:  "codex",
		Status:    "running",
		UpdatedAt: "2026-07-01T10:00:01Z",
	}

	w.prevAgents[agent.ID] = &AgentState{
		LastUpdatedAt: "2026-07-01T10:00:00Z",
		StuckSince:    "2026-07-01T09:00:00Z",
		StuckNotified: true,
	}

	w.detectStuckAgents([]AgentStatus{agent})

	state := w.prevAgents[agent.ID]
	if state.StuckSince != "" {
		t.Fatalf("StuckSince should be reset when UpdatedAt changes, got %s", state.StuckSince)
	}
	if state.StuckNotified {
		t.Fatalf("StuckNotified should be false when UpdatedAt changes")
	}
	if state.LastUpdatedAt != "2026-07-01T10:00:01Z" {
		t.Fatalf("LastUpdatedAt should be updated to new value, got %s", state.LastUpdatedAt)
	}
}

func TestStuckResetOnEmptyUpdatedAt(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

	agent := AgentStatus{
		ID:        "agent-stuck-4",
		ShortID:   "agent-stuck-4",
		Title:     "running task",
		Provider:  "codex",
		Status:    "running",
		UpdatedAt: "",
	}

	w.prevAgents[agent.ID] = &AgentState{
		LastUpdatedAt: "2026-07-01T10:00:00Z",
		StuckSince:    "2026-07-01T09:00:00Z",
		StuckNotified: true,
	}

	w.detectStuckAgents([]AgentStatus{agent})

	state := w.prevAgents[agent.ID]
	if state.StuckSince != "" {
		t.Fatalf("StuckSince should be reset when UpdatedAt is empty, got %s", state.StuckSince)
	}
	if state.StuckNotified {
		t.Fatalf("StuckNotified should be false when UpdatedAt is empty")
	}
}

func TestStuckNotifiedSkip(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)
	w.SetStuckDetectTimeout(1)

	agent := AgentStatus{
		ID:        "agent-stuck-5",
		ShortID:   "agent-stuck-5",
		Title:     "running task",
		Provider:  "codex",
		Status:    "running",
		UpdatedAt: "2026-07-01T10:00:00Z",
	}

	w.prevAgents[agent.ID] = &AgentState{
		LastUpdatedAt: agent.UpdatedAt,
		StuckSince:    time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		StuckNotified: true,
	}

	w.detectStuckAgents([]AgentStatus{agent})

	if len(notifier.events) != 0 {
		t.Fatalf("already notified stuck agent should not trigger again, got %d events", len(notifier.events))
	}
}

func TestStuckNoPrevState(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)
	w.SetStuckDetectTimeout(1)

	agent := AgentStatus{
		ID:        "agent-stuck-6",
		ShortID:   "agent-stuck-6",
		Title:     "new agent",
		Provider:  "codex",
		Status:    "running",
		UpdatedAt: "2026-07-01T10:00:00Z",
	}

	w.detectStuckAgents([]AgentStatus{agent})

	if len(notifier.events) != 0 {
		t.Fatalf("agent with no prev state should not trigger stuck event, got %d", len(notifier.events))
	}
}
