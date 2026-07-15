package agentwatcher

import (
	"testing"
	"time"

	"github.com/winezer0/paseo-notifier/config"
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

func TestDetectCancelledEvent(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)

	agent1 := AgentStatus{
		ID:              "agent-cancel-1",
		ShortID:         "agent-cancel-1",
		Title:           "取消任务",
		Provider:        "codex",
		Status:          "running",
		AttentionReason: nil,
	}

	agent2 := AgentStatus{
		ID:                "agent-cancel-1",
		ShortID:           "agent-cancel-1",
		Title:             "取消任务",
		Provider:          "codex",
		Status:            "idle",
		RequiresAttention: true,
		AttentionReason:   strPtr("cancelled"),
	}

	// 第一次扫描：无 attentionReason → 记录状态，不触发事件
	w.detectAgentChange(agent1)
	if len(notifier.events) != 0 {
		t.Fatalf("first scan should record state, not trigger event, got %d events", len(notifier.events))
	}

	// 第二次扫描：attentionReason=cancelled → 更新状态，不发送通知
	w.detectAgentChange(agent2)
	if len(notifier.events) != 0 {
		t.Fatalf("cancelled event should not send notification, got %d events", len(notifier.events))
	}

	// 第三次扫描：相同 cancelled 状态 → 不重复触发
	w.detectAgentChange(agent2)
	if len(notifier.events) != 0 {
		t.Fatalf("duplicate cancelled should not trigger again, got %d events", len(notifier.events))
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
	notifier := &mockNotifier{done: make(chan struct{}, 10)}
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
	<-notifier.done // 等待异步通知完成

	if len(notifier.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(notifier.events))
	}
	if notifier.events[0].Type != EventPermissionRequest {
		t.Fatalf("expected EventPermissionRequest, got %s", notifier.events[0].Type)
	}
}

func TestSkipDuplicatePermission(t *testing.T) {
	notifier := &mockNotifier{done: make(chan struct{}, 10)}
	w := testWatcher(notifier)

	perm1 := PermissionRequest{
		AgentID: "agent-5",
		Status:  "running",
	}
	perm1.Request.ID = "perm-2"
	perm1.Request.Kind = "tool"

	w.detectNewPermission(perm1)
	<-notifier.done // 等待第一个异步通知
	w.detectNewPermission(perm1)
	// 第二个是重复的，不会发通知

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

// ─────────────── auto continue colon detection tests ───────────────

func TestHasTrailingColon(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"zh colon", "以下方案：", true},
		{"en colon", "Here are the items:", true},
		{"zh colon with space", "以下方案：  \n", true},
		{"en colon trailing space", "items:   ", true},
		{"no colon", "任务已完成。", false},
		{"empty", "", false},
		{"only spaces", "   ", false},
		{"comma ending", "第一，第二，", false},
		{"period ending", "done.", false},
		{"question mark", "是否继续？", false},
		{"colon in middle", "note: done", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTrailingColon(tt.input)
			if got != tt.expect {
				t.Errorf("hasTrailingColon(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

// ─────────────── notify_min_duration tests ───────────────

func TestSuppressShortTaskNotification(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher(config.MonitorConfig{
		DaemonURL:          "http://localhost:9999",
		Interval:           "1s",
		StuckDetectTimeout: "120s",
		NotifyMinDuration:  "30s",
	}, notifier, "继续任务", "卡死恢复提示", "子任务已完成，请继续主任务。")

	// UpdatedAt 距现在仅 10 秒时完成 → 应被抑制
	updatedAt := time.Now().Add(-10 * time.Second).Format(time.RFC3339)
	agent := AgentStatus{
		ID:              "agent-short",
		ShortID:         "agent-short",
		Title:           "短任务",
		AttentionReason: nil,
		UpdatedAt:       updatedAt,
	}
	w.detectAgentChange(agent)

	agent.AttentionReason = strPtr("finished")
	w.detectAgentChange(agent)

	if len(notifier.events) != 0 {
		t.Fatalf("short task notification should be suppressed, got %d events", len(notifier.events))
	}
}

func TestAllowLongTaskNotification(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher(config.MonitorConfig{
		DaemonURL:          "http://localhost:9999",
		Interval:           "1s",
		StuckDetectTimeout: "120s",
		NotifyMinDuration:  "30s",
	}, notifier, "继续任务", "卡死恢复提示", "子任务已完成，请继续主任务。")

	// UpdatedAt 距现在 60 秒时完成 → 应正常通知
	updatedAt := time.Now().Add(-60 * time.Second).Format(time.RFC3339)
	agent := AgentStatus{
		ID:              "agent-long",
		ShortID:         "agent-long",
		Title:           "长任务",
		AttentionReason: nil,
		UpdatedAt:       updatedAt,
	}
	w.detectAgentChange(agent)

	agent.AttentionReason = strPtr("finished")
	w.detectAgentChange(agent)

	if len(notifier.events) != 1 {
		t.Fatalf("long task notification should be sent, got %d events", len(notifier.events))
	}
	if notifier.events[0].Type != EventFinished {
		t.Fatalf("expected EventFinished, got %s", notifier.events[0].Type)
	}
}

func TestSuppressDisabledWhenZero(t *testing.T) {
	notifier := &mockNotifier{}
	w := NewWatcher(config.MonitorConfig{
		DaemonURL:          "http://localhost:9999",
		Interval:           "1s",
		StuckDetectTimeout: "120s",
		NotifyMinDuration:  "0s",
	}, notifier, "继续任务", "卡死恢复提示", "子任务已完成，请继续主任务。")

	// UpdatedAt 距现在仅 1 秒 → notify_min_duration=0 不抑制
	updatedAt := time.Now().Add(-1 * time.Second).Format(time.RFC3339)
	agent := AgentStatus{
		ID:              "agent-zero",
		ShortID:         "agent-zero",
		Title:           "极短任务",
		AttentionReason: nil,
		UpdatedAt:       updatedAt,
	}
	w.detectAgentChange(agent)

	agent.AttentionReason = strPtr("finished")
	w.detectAgentChange(agent)

	if len(notifier.events) != 1 {
		t.Fatalf("notify_min_duration=0 should not suppress, got %d events", len(notifier.events))
	}
}

func TestShouldAutoContinueByColon(t *testing.T) {
	notifier := &mockNotifier{}
	w := testWatcher(notifier)
	w.autoContinue = true
	w.continuePrompt = "继续任务"

	tests := []struct {
		name    string
		summary string
		want    bool
	}{
		{"zh colon interrupted", "以下是我整理的分析结果：", true},
		{"en colon interrupted", "Here are the suggestions:", true},
		{"continue keyword zh", "是否继续？", true},
		{"continue keyword en", "Shall I continue?", true},
		{"normal finished", "任务已完成。", false},
		{"listing completed", "以上是全部内容。", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []ActivityEntry{
				{Timestamp: "2026-07-01T10:00:00Z", Type: "tool_call", Summary: tt.summary},
			}
			got := w.shouldAutoContinue(entries, "test-agent")
			if got != tt.want {
				t.Errorf("shouldAutoContinue(%q) = %v, want %v", tt.summary, got, tt.want)
			}
		})
	}
}
