package agentwatcher

import (
	"testing"
)

func TestParseSubagentFromActivity(t *testing.T) {
	tests := []struct {
		name     string
		entry    ActivityEntry
		wantID   string
		wantKind SubagentKind
		wantNil  bool
	}{
		{
			name: "empty summary returns nil",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T03:51:21Z",
				Type:      "tool",
				Summary:   "",
			},
			wantNil: true,
		},
		{
			name: "task result with bg_xxx ID",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T04:02:16Z",
				Type:      "tool",
				Summary:   "Task Result\n\nTask ID: bg_746aea36\nDescription: Fix AddNode cols: python+php+java\nDuration: 10m 55s\nSession ID: ses_0b5db2454ffe8ExN84O83mPodg",
			},
			wantID:   "bg_746aea36",
			wantKind: SubagentOpenCode,
		},
		{
			name: "task result minimal fields",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T04:02:16Z",
				Type:      "tool",
				Summary:   "Task ID: bg_abc123",
			},
			wantID:   "bg_abc123",
			wantKind: SubagentOpenCode,
		},
		{
			name: "activity with bg_ but no subagent context returns nil",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T03:51:21Z",
				Type:      "thought",
				Summary:   "processing bg_ pattern match",
			},
			wantNil: true,
		},
		{
			name: "activity with bg_ and subagent keywords",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T03:51:21Z",
				Type:      "tool",
				Summary:   "4 parallel subagents launched: Task ID: bg_3dc26f2e",
			},
			wantID:   "bg_3dc26f2e",
			wantKind: SubagentOpenCode,
		},
		{
			name: "unrelated activity returns nil",
			entry: ActivityEntry{
				Timestamp: "2026-07-10T03:51:21Z",
				Type:      "thought",
				Summary:   "I need to check the file for errors",
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubagentFromActivity(tt.entry)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseSubagentFromActivity() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("parseSubagentFromActivity() returned nil, want non-nil")
			}
			if got.ID != tt.wantID {
				t.Errorf("got.ID = %q, want %q", got.ID, tt.wantID)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("got.Kind = %q, want %q", got.Kind, tt.wantKind)
			}
		})
	}
}

func TestParseSubagentFromActivityTaskResult(t *testing.T) {
	entry := ActivityEntry{
		Timestamp: "2026-07-10T04:02:16Z",
		Type:      "tool",
		Summary: "Task Result\n\nTask ID: bg_746aea36\n" +
			"Description: Fix AddNode cols: python+php+java\n" +
			"Duration: 10m 55s\n" +
			"Session ID: ses_0b5db2454ffe8ExN84O83mPodg",
	}

	got := parseSubagentFromActivity(entry)
	if got == nil {
		t.Fatal("parseSubagentFromActivity() returned nil")
	}

	if got.Description != "Fix AddNode cols: python+php+java" {
		t.Errorf("got.Description = %q, want %q", got.Description, "Fix AddNode cols: python+php+java")
	}
	if got.Duration != "10m 55s" {
		t.Errorf("got.Duration = %q, want %q", got.Duration, "10m 55s")
	}
	if got.SessionID != "ses_0b5db2454ffe8ExN84O83mPodg" {
		t.Errorf("got.SessionID = %q, want %q", got.SessionID, "ses_0b5db2454ffe8ExN84O83mPodg")
	}
	if got.Status != "completed" {
		t.Errorf("got.Status = %q, want %q", got.Status, "completed")
	}
}

func TestDetectChangesNewSubagent(t *testing.T) {
	tracker := NewSubagentTracker()

	entries := []ActivityEntry{
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "4 parallel subagents launched: Task ID: bg_3dc26f2e",
		},
	}

	// running agent（allowNew=true）首次遇到（基线）也会返回 changes
	changes := tracker.DetectChanges("parent-1", entries, true)
	if len(changes) != 1 {
		t.Fatalf("DetectChanges() returned %d changes, want 1", len(changes))
	}
	if changes[0].ID != "bg_3dc26f2e" {
		t.Errorf("changes[0].ID = %q, want %q", changes[0].ID, "bg_3dc26f2e")
	}
	if changes[0].ParentID != "parent-1" {
		t.Errorf("changes[0].ParentID = %q, want %q", changes[0].ParentID, "parent-1")
	}
	if changes[0].Status != "running" {
		t.Errorf("changes[0].Status = %q, want %q", changes[0].Status, "running")
	}
}

func TestDetectChangesMultipleNew(t *testing.T) {
	tracker := NewSubagentTracker()

	entries := []ActivityEntry{
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "parallel subagents: Task ID: bg_3dc26f2e",
		},
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "parallel subagents: Task ID: bg_2fce6ead",
		},
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "parallel subagents: Task ID: bg_746aea36",
		},
	}

	changes := tracker.DetectChanges("parent-1", entries, true)
	if len(changes) != 3 {
		t.Fatalf("DetectChanges() returned %d changes, want 3", len(changes))
	}
}

func TestDetectChangesNoChange(t *testing.T) {
	tracker := NewSubagentTracker()

	entries := []ActivityEntry{
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "Task ID: bg_3dc26f2e",
		},
	}

	// running agent（allowNew=true）首次遇到返回 changes
	changes1 := tracker.DetectChanges("parent-1", entries, true)
	if len(changes1) != 1 {
		t.Fatalf("first DetectChanges() = %d, want 1", len(changes1))
	}

	// 第二次相同条目，无变化
	changes2 := tracker.DetectChanges("parent-1", entries, true)
	if changes2 != nil {
		t.Errorf("second DetectChanges() = %v, want nil", changes2)
	}
}

func TestDetectChangesStatusChange(t *testing.T) {
	tracker := NewSubagentTracker()

	tracker.DetectChanges("parent-1", []ActivityEntry{
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "subagents launched: Task ID: bg_3dc26f2e",
		},
	}, true)

	changes := tracker.DetectChanges("parent-1", []ActivityEntry{
		{
			Timestamp: "2026-07-10T04:02:16Z",
			Type:      "tool",
			Summary: "Task Result\n\nTask ID: bg_3dc26f2e\n" +
				"Description: Fix parsing\n" +
				"Duration: 5m 30s",
		},
	}, true)

	if len(changes) != 1 {
		t.Fatalf("DetectChanges() = %d changes, want 1", len(changes))
	}
	if changes[0].Status != "completed" {
		t.Errorf("changes[0].Status = %q, want %q", changes[0].Status, "completed")
	}
}

func TestDetectChangesDisappeared(t *testing.T) {
	tracker := NewSubagentTracker()

	tracker.DetectChanges("parent-1", []ActivityEntry{
		{
			Timestamp: "2026-07-10T03:51:21Z",
			Type:      "tool",
			Summary:   "subagents: Task ID: bg_3dc26f2e",
		},
	}, true)

	changes := tracker.DetectChanges("parent-1", []ActivityEntry{
		{
			Timestamp: "2026-07-10T04:02:16Z",
			Type:      "thought",
			Summary:   "task is done",
		},
	}, true)

	if len(changes) != 1 {
		t.Fatalf("DetectChanges() = %d changes, want 1", len(changes))
	}
	if changes[0].ID != "bg_3dc26f2e" {
		t.Errorf("changes[0].ID = %q, want %q", changes[0].ID, "bg_3dc26f2e")
	}
	if changes[0].Status != "completed" {
		t.Errorf("changes[0].Status = %q, want %q", changes[0].Status, "completed")
	}
}

func TestGetAll(t *testing.T) {
	tracker := NewSubagentTracker()

	tracker.DetectChanges("parent-1", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_1"},
		{Summary: "subagents: Task ID: bg_2"},
	}, true)

	all := tracker.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll() = %d, want 2", len(all))
	}
}

func TestGetByParent(t *testing.T) {
	tracker := NewSubagentTracker()

	tracker.DetectChanges("parent-1", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_1"},
	}, true)
	tracker.DetectChanges("parent-2", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_2"},
	}, true)

	children := tracker.GetByParent("parent-1")
	if len(children) != 1 {
		t.Errorf("GetByParent(parent-1) = %d, want 1", len(children))
	}
	if len(children) > 0 && children[0].ID != "bg_1" {
		t.Errorf("children[0].ID = %q, want %q", children[0].ID, "bg_1")
	}

	children2 := tracker.GetByParent("parent-2")
	if len(children2) != 1 {
		t.Errorf("GetByParent(parent-2) = %d, want 1", len(children2))
	}
}

func TestReset(t *testing.T) {
	tracker := NewSubagentTracker()
	tracker.DetectChanges("parent-1", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_1"},
	}, true)

	if len(tracker.GetAll()) != 1 {
		t.Errorf("before reset: GetAll() = %d, want 1", len(tracker.GetAll()))
	}

	tracker.Reset()

	if len(tracker.GetAll()) != 0 {
		t.Errorf("after reset: GetAll() = %d, want 0", len(tracker.GetAll()))
	}
}

func TestDetectChangesDifferentParents(t *testing.T) {
	tracker := NewSubagentTracker()

	// 两个不同的 running agent 首次各自返回 changes
	c1 := tracker.DetectChanges("parent-1", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_parent1_task"},
	}, true)
	c2 := tracker.DetectChanges("parent-2", []ActivityEntry{
		{Summary: "subagents: Task ID: bg_parent2_task"},
	}, true)

	if len(c1) != 1 {
		t.Errorf("parent-1 changes = %d, want 1", len(c1))
	}
	if c1[0].ParentID != "parent-1" {
		t.Errorf("c1 ParentID = %q, want %q", c1[0].ParentID, "parent-1")
	}
	if len(c2) != 1 {
		t.Errorf("parent-2 changes = %d, want 1", len(c2))
	}
	if c2[0].ParentID != "parent-2" {
		t.Errorf("c2 ParentID = %q, want %q", c2[0].ParentID, "parent-2")
	}
}

func TestParseActivityTextDump(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     int
		wantFirst string
	}{
		{
			name:  "empty text returns nil",
			input: "",
			want:  0,
		},
		{
			name: "standard format with multiple entries",
			input: "Showing all 1077 activities\n\n" +
				"[User] analyze basetypes/node.go\n" +
				"[Thought] checking the struct\n" +
				"[Read] file.go\n" +
				"[general] sleep 30s test\n",
			want:      4,
			wantFirst: "analyze basetypes/node.go",
		},
		{
			name: "format with count instead of 'all'",
			input: "Showing 3 of 417 activities\n\n" +
				"[Read] main.go\n" +
				"[Thought] processing\n" +
				"[Paseo Get Agent Activity] agent-123\n",
			want:      3,
			wantFirst: "main.go",
		},
		{
			name: "lines without bracket prefix are skipped",
			input: "Showing 5 activities\n\n" +
				"[Read] file.go\n" +
				"some random text\n" +
				"   \n" +
				"[Thought] checking\n",
			want:      2,
			wantFirst: "file.go",
		},
		{
			name: "task result multi-line captured as buffer",
			input: "Showing 5 activities\n\n" +
				"[Read] file.go\n" +
				"Task Result\n" +
				"Task ID: bg_abc123\n" +
				"Duration: 41s\n" +
				"[Thought] done\n",
			want:      3,
			wantFirst: "file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseActivityTextDump(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseActivityTextDump() returned %d entries, want %d", len(got), tt.want)
			}
			if tt.wantFirst != "" && len(got) > 0 && got[0].Summary != tt.wantFirst {
				t.Errorf("got[0].Summary = %q, want %q", got[0].Summary, tt.wantFirst)
			}
			if len(got) > 0 && got[0].Type == "" && tt.wantFirst != "" {
				t.Errorf("got[0].Type is empty, should be extracted from [Type] prefix")
			}
		})
	}
}

func TestParseActivityLine(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantSumm string
	}{
		{"[Read] file.go", "Read", "file.go"},
		{"[Thought] checking", "Thought", "checking"},
		{"[general] sleep 30s", "general", "sleep 30s"},
		{"[Paseo Get Agent Activity] agent-123", "Paseo Get Agent Activity", "agent-123"},
		{"[User] what is this?", "User", "what is this?"},
		{"no bracket", "", "no bracket"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			typ, summ := parseActivityLine(tt.input)
			if typ != tt.wantType {
				t.Errorf("parseActivityLine() type = %q, want %q", typ, tt.wantType)
			}
			if summ != tt.wantSumm {
				t.Errorf("parseActivityLine() summary = %q, want %q", summ, tt.wantSumm)
			}
		})
	}
}

func TestParseSubagentFromActivityNonBg(t *testing.T) {
	entries := []ActivityEntry{
		{Summary: "[Read] file.go"},
		{Summary: "[Thought] checking logic"},
		{Summary: "[Session Search] find something"},
		{Summary: "[Paseo Get Agent Activity] agent-123"},
	}

	for _, entry := range entries {
		t.Run(entry.Summary, func(t *testing.T) {
			got := parseSubagentFromActivity(entry)
			if got != nil {
				t.Errorf("parseSubagentFromActivity() = %v, want nil for summary=%q", got, entry.Summary)
			}
		})
	}
}
