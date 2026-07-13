package agentwatcher

import (
	"encoding/json"
	"testing"
)

// makeUpdatePayload 构造 "agent.provider_subagents.update" 消息的 JSON 载荷（匹配 daemon 真实格式）
func makeUpdatePayload(parentID, subID, title, provider, status string) json.RawMessage {
	update := providerSubagentUpdate{
		Kind: "upsert",
		Subagent: &providerSubagentPayload{
			ID:            subID,
			ParentAgentID: parentID,
			Title:         title,
			Provider:      provider,
			Status:        status,
		},
	}
	data, _ := json.Marshal(update)
	return data
}

// makeListPayload 构造 "agent.provider_subagents.list.response" 消息的 JSON 载荷
func makeListPayload(subagents ...providerSubagentPayload) json.RawMessage {
	resp := providerSubagentStatusListResponse{Subagents: subagents}
	data, _ := json.Marshal(resp)
	return data
}

func TestProviderTrackerSingleComplete(t *testing.T) {
	var gotParentID string
	var gotSubagents []ProviderSubagentStatus
	tracker := NewProviderSubagentTracker(
		func(parentID string, subagents []ProviderSubagentStatus) {
			gotParentID = parentID
			gotSubagents = subagents
		}, nil)

	// 新增 running 子任务 → 不触发全部完成
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "build auth", "codex", "running"))
	if gotParentID != "" {
		t.Fatal("should not trigger all-done for running subagent")
	}

	// 标记 completed → 唯一子任务完成，触发全部完成
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "build auth", "codex", "completed"))
	if gotParentID != "agent-1" {
		t.Fatalf("expected parentID='agent-1', got %q", gotParentID)
	}
	if len(gotSubagents) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(gotSubagents))
	}
	if gotSubagents[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", gotSubagents[0].Status)
	}
}

func TestProviderTrackerMultipleAllCompleted(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	// 3 个子任务
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-3", "task C", "opencode", "running"))

	if callCount != 0 {
		t.Fatal("should not trigger all-done yet")
	}

	// 逐个完成
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "completed"))

	if callCount != 0 {
		t.Fatal("should not trigger all-done until last one completes")
	}

	// 最后一个完成 → 触发
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-3", "task C", "opencode", "completed"))
	if callCount != 1 {
		t.Fatalf("expected 1 all-done call, got %d", callCount)
	}
}

func TestProviderTrackerNoDuplicateNotification(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))

	if callCount != 1 {
		t.Fatalf("expected 1 all-done call, got %d", callCount)
	}

	// 重复 completed → 不触发
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 1 {
		t.Fatalf("duplicate completed should not trigger again, got %d calls", callCount)
	}
}

func TestProviderTrackerNewSubagentResetsFlag(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 1 {
		t.Fatalf("first round all-done: expected 1, got %d", callCount)
	}

	// 新增子任务 → 重置标记
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "running"))

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "completed"))
	if callCount != 2 {
		t.Fatalf("second round all-done: expected 2, got %d", callCount)
	}
}

func TestProviderTrackerSubagentRevived(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 1 {
		t.Fatalf("first all-done: expected 1, got %d", callCount)
	}

	// running 复活 → 重置标记
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))

	// 再次完成 → 重新触发
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 2 {
		t.Fatalf("revived all-done: expected 2, got %d", callCount)
	}
}

func TestProviderTrackerErrorStatus(t *testing.T) {
	var gotParentID string
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		gotParentID = parentID
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "error"))

	if gotParentID != "agent-1" {
		t.Fatal("error status should trigger all-done")
	}
}

func TestProviderTrackerIdleStatus(t *testing.T) {
	var gotParentID string
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		gotParentID = parentID
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "idle"))

	if gotParentID != "agent-1" {
		t.Fatal("idle status should trigger all-done")
	}
}

func TestProviderTrackerNoSubagents(t *testing.T) {
	var called bool
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		called = true
	}, nil)

	tracker.HandleSubagentUpdate(json.RawMessage(`{}`))
	if called {
		t.Fatal("no subagents should not trigger all-done")
	}
}

func TestProviderTrackerReset(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 1 {
		t.Fatalf("before reset: expected 1, got %d", callCount)
	}

	tracker.Reset()

	// Reset 不清空 allDoneNotified → 重连后 list.response 加载已完成 subagent 不重复通知
	tracker.HandleSubagentList(makeListPayload(
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-1", Title: "task A", Provider: "codex", Status: "completed"},
	))
	if callCount != 1 {
		t.Fatalf("after reset+reload: allDoneNotified persisted, expected still 1, got %d", callCount)
	}
}

func TestProviderTrackerResetNewSubagentAfterReset(t *testing.T) {
	var callCount int
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		callCount++
	}, nil)

	// 第一轮完成 → 通知
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "completed"))
	if callCount != 1 {
		t.Fatalf("first round: expected 1, got %d", callCount)
	}

	tracker.Reset()

	// Reset 后出现新 subagent → 应重新触发（因为新 subagent 重置了 allDoneNotified）
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "completed"))
	if callCount != 2 {
		t.Fatalf("new subagent after reset: expected 2, got %d", callCount)
	}
}

func TestProviderTrackerGetByParent(t *testing.T) {
	tracker := NewProviderSubagentTracker(nil, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "completed"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-2", "sub-3", "task C", "opencode", "running"))

	subs := tracker.GetByParent("agent-1")
	if len(subs) != 2 {
		t.Fatalf("agent-1 should have 2 subagents, got %d", len(subs))
	}

	subs2 := tracker.GetByParent("agent-nonexistent")
	if len(subs2) != 0 {
		t.Fatalf("nonexistent parent should have 0 subagents, got %d", len(subs2))
	}
}

func TestProviderTrackerListResponse(t *testing.T) {
	var gotParentID string
	var gotSubagents []ProviderSubagentStatus
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		gotParentID = parentID
		gotSubagents = subagents
	}, nil)

	payload := makeListPayload(
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-1", Title: "task A", Provider: "codex", Status: "completed"},
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-2", Title: "task B", Provider: "codex", Status: "completed"},
	)
	tracker.HandleSubagentList(payload)

	if gotParentID != "agent-1" {
		t.Fatalf("list with all completed should trigger: got %q", gotParentID)
	}
	if len(gotSubagents) != 2 {
		t.Fatalf("expected 2 subagents, got %d", len(gotSubagents))
	}
}

func TestProviderTrackerListResponsePartialComplete(t *testing.T) {
	var called bool
	tracker := NewProviderSubagentTracker(func(parentID string, subagents []ProviderSubagentStatus) {
		called = true
	}, nil)

	payload := makeListPayload(
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-1", Title: "task A", Provider: "codex", Status: "completed"},
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-2", Title: "task B", Provider: "codex", Status: "running"},
	)
	tracker.HandleSubagentList(payload)

	if called {
		t.Fatal("partial complete should not trigger all-done")
	}
}

// ─────────────── 新增测试：onFirstSubagent ───────────────

func TestProviderTrackerFirstSubagent(t *testing.T) {
	var spawnedParent string
	var spawnedSub ProviderSubagentStatus
	tracker := NewProviderSubagentTracker(nil, func(parentID string, sa ProviderSubagentStatus) {
		spawnedParent = parentID
		spawnedSub = sa
	})

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "build auth", "codex", "running"))
	if spawnedParent != "agent-1" {
		t.Fatalf("expected first subagent spawn for agent-1, got %q", spawnedParent)
	}
	if spawnedSub.SubagentID != "sub-1" {
		t.Fatalf("expected sub-1, got %q", spawnedSub.SubagentID)
	}
}

func TestProviderTrackerFirstSubagentOnlyOnce(t *testing.T) {
	var spawnCount int
	tracker := NewProviderSubagentTracker(nil, func(parentID string, sa ProviderSubagentStatus) {
		spawnCount++
	})

	// 第一个 → 触发
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	if spawnCount != 1 {
		t.Fatalf("first spawn: expected 1, got %d", spawnCount)
	}

	// 第二个 → 不触发（不是首次了）
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "running"))
	if spawnCount != 1 {
		t.Fatalf("second should not trigger spawn: expected 1, got %d", spawnCount)
	}
}

func TestProviderTrackerFirstSubagentDifferentParents(t *testing.T) {
	var spawnedParents []string
	tracker := NewProviderSubagentTracker(nil, func(parentID string, sa ProviderSubagentStatus) {
		spawnedParents = append(spawnedParents, parentID)
	})

	// 不同父 agent 各自触发
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-2", "sub-1", "task B", "codex", "running"))

	if len(spawnedParents) != 2 {
		t.Fatalf("expected 2 spawns for different parents, got %d", len(spawnedParents))
	}
}

func TestProviderTrackerResetClearsSpawnFlag(t *testing.T) {
	var spawnCount int
	tracker := NewProviderSubagentTracker(nil, func(parentID string, sa ProviderSubagentStatus) {
		spawnCount++
	})

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	if spawnCount != 1 {
		t.Fatalf("before reset: expected 1 spawn, got %d", spawnCount)
	}

	tracker.Reset()

	// Reset 不清空 spawnNotified → 重连后 list.response 加载已有 subagent 不触发 spawn
	tracker.HandleSubagentList(makeListPayload(
		providerSubagentPayload{ParentAgentID: "agent-1", ID: "sub-1", Title: "task A", Provider: "codex", Status: "completed"},
	))
	if spawnCount != 1 {
		t.Fatalf("after reset+reload: spawnNotified persisted, expected still 1, got %d", spawnCount)
	}
}

// ─────────────── 新增测试：GetTrackedParents / HasRunningSubagents ───────────────

func TestProviderTrackerGetTrackedParents(t *testing.T) {
	tracker := NewProviderSubagentTracker(nil, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-2", "sub-2", "task B", "codex", "completed"))

	parents := tracker.GetTrackedParents()
	if len(parents) != 2 {
		t.Fatalf("expected 2 tracked parents, got %d", len(parents))
	}
}

func TestProviderTrackerHasRunningSubagents(t *testing.T) {
	tracker := NewProviderSubagentTracker(nil, nil)

	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-1", "task A", "codex", "running"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-1", "sub-2", "task B", "codex", "completed"))
	tracker.HandleSubagentUpdate(makeUpdatePayload("agent-2", "sub-3", "task C", "codex", "completed"))

	if !tracker.HasRunningSubagents("agent-1") {
		t.Fatal("agent-1 should have running subagents")
	}
	if tracker.HasRunningSubagents("agent-2") {
		t.Fatal("agent-2 should NOT have running subagents")
	}
	if tracker.HasRunningSubagents("agent-nonexistent") {
		t.Fatal("nonexistent agent should NOT have running subagents")
	}
}

// ─────────────── 原有测试：FormatSubagentSummary / SubagentStatusLabel ───────────────

func TestFormatSubagentSummary(t *testing.T) {
	tests := []struct {
		name      string
		subagents []ProviderSubagentStatus
		want      string
	}{
		{name: "empty", subagents: nil, want: "0 total"},
		{
			name: "all running",
			subagents: []ProviderSubagentStatus{
				{Status: "running"}, {Status: "running"},
			},
			want: "2 running (2 total)",
		},
		{
			name: "all completed",
			subagents: []ProviderSubagentStatus{
				{Status: "completed"}, {Status: "completed"}, {Status: "completed"},
			},
			want: "3 completed (3 total)",
		},
		{
			name: "mixed",
			subagents: []ProviderSubagentStatus{
				{Status: "running"}, {Status: "completed"}, {Status: "error"}, {Status: "idle"},
			},
			want: "1 running, 1 completed, 1 error, 1 idle (4 total)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSubagentSummary(tt.subagents)
			if got != tt.want {
				t.Errorf("FormatSubagentSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubagentStatusLabel(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"running", "运行中"},
		{"completed", "已完成"},
		{"error", "出错"},
		{"idle", "空闲"},
		{"unknown_status", "unknown_status"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := SubagentStatusLabel(tt.status)
			if got != tt.want {
				t.Errorf("SubagentStatusLabel(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
