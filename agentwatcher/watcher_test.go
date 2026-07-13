package agentwatcher

import (
	"context"
	"testing"

	"github.com/winezer0/paseo-notifier/config"
)

// testWatcher 创建一个测试用 Watcher（不连接真实 daemon）
func testWatcher(notifier *mockNotifier) *Watcher {
	return NewWatcher(config.MonitorConfig{
		DaemonURL:          "http://localhost:9999",
		Interval:           "1s",
		StuckDetectTimeout: "120s",
		StuckRestartRetry:  5,
	}, notifier, "继续任务", "检测到 Agent 长时间无响应，请检查你的执行状态，从之前的工作继续。如果你不记得之前的任务，请重新询问用户。", "子任务已完成，请继续主任务。")
}

// mockNotifier 模拟通知器，记录接收的事件
type mockNotifier struct {
	events []AgentEvent
	done   chan struct{} // 非空时通知测试 goroutine 已接收事件
}

func (m *mockNotifier) Notify(ctx context.Context, event AgentEvent) error {
	m.events = append(m.events, event)
	if m.done != nil {
		m.done <- struct{}{}
	}
	return nil
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

func strPtr(s string) *string {
	return &s
}

func TestHandleConnStateInitialDisconnect(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := testWatcher(notifier)
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
	w := testWatcher(notifier)
	w.SetSystemNotifier(sys.notify)

	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("expected 1 disconnect notification, got %d", len(sys.disconnected))
	}

	w.prevAgents["test"] = &AgentState{
		AttentionReason: strPtr("finished"),
	}
	w.prevPermIDs["test/perm-1"] = true

	w.handleConnState(false)

	if w.connState != ConnConnected {
		t.Fatalf("after reconnect, state should be connected, got %s", w.connState)
	}
	if len(sys.disconnected) != 2 {
		t.Fatalf("expected 2 system notifications (disconnect + reconnect), got %d", len(sys.disconnected))
	}
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
	w := testWatcher(notifier)
	w.SetSystemNotifier(sys.notify)

	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("expected 1 notification after first disconnect, got %d", len(sys.disconnected))
	}

	w.handleConnState(true)
	if len(sys.disconnected) != 1 {
		t.Fatalf("continuous disconnect should not repeat notification, got %d", len(sys.disconnected))
	}
}

func TestHandleConnStateContinuousConnectedDoesNotNotify(t *testing.T) {
	notifier := &mockNotifier{}
	sys := &mockSysNotifier{}
	w := testWatcher(notifier)
	w.SetSystemNotifier(sys.notify)

	w.handleConnState(false)
	if len(sys.disconnected) != 0 {
		t.Fatalf("continuous connected should not send notifications, got %d", len(sys.disconnected))
	}
}

func TestNormalizeDaemonURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"already base", "http://127.0.0.1:6767", "http://127.0.0.1:6767"},
		{"with mcp suffix", "http://127.0.0.1:6767/mcp/agents", "http://127.0.0.1:6767"},
		{"with port and suffix", "http://localhost:6767/mcp/agents", "http://localhost:6767"},
		{"trailing slash no mcp", "http://127.0.0.1:6767/", "http://127.0.0.1:6767/"},
		{"only suffix no match", "http://127.0.0.1:6767/mcp", "http://127.0.0.1:6767/mcp"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDaemonURL(tt.raw)
			if got != tt.want {
				t.Errorf("normalizeDaemonURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}