package agentwatcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 通过真实子任务回调验证：父任务不在列表内时，仍应按 ID 取得完整信息。
func TestSubagentParentOutsideList(t *testing.T) {
	const parent = "c6dfda07-1ecb-4256-8b4a-377815d61d1d"
	for _, event := range []string{"running", "completed", "heartbeat"} {
		t.Run(event, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Params struct {
						Name      string            `json:"name"`
						Arguments map[string]string `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if req.Params.Name == "list_agents" {
					fmt.Fprint(w, "data: {\"result\":{\"structuredContent\":{\"agents\":[]}}}\n\n")
					return
				}
				calls++
				if req.Params.Name != "get_agent_status" || req.Params.Arguments["agentId"] != parent {
					t.Errorf("unexpected request: %+v", req)
				}
				fmt.Fprintf(w, "data: {\"result\":{\"structuredContent\":{\"snapshot\":{\"id\":%q,\"title\":\"父任务\",\"model\":\"gpt-6-astra\",\"cwd\":\"/work\",\"status\":\"running\"}}}}\n\n", parent)
			}))
			defer server.Close()
			n := &mockNotifier{}
			w := testWatcher(n)
			defer w.cancel()
			w.mcpURL = server.URL
			w.setupSubagentTracking()
			status := event
			if event == "heartbeat" {
				status = "running"
			}
			w.subagentTracker.HandleSubagentUpdate(makeUpdatePayload(parent, "child", "Standards", "codex", status))
			if event == "heartbeat" {
				n.events = nil
				n.done = make(chan struct{}, 1)
				w.subagentRunningInterval = time.Second
				w.mu.Lock()
				delete(w.lastSubagentNotify, parent)
				w.mu.Unlock()
				w.checkRunningSubagents()
				select {
				case <-n.done:
				case <-time.After(time.Second):
					t.Fatal("heartbeat notification timed out")
				}
			}
			if calls == 0 || len(n.events) == 0 {
				t.Fatalf("lookup calls=%d notifications=%d", calls, len(n.events))
			}
			for _, e := range n.events {
				if e.Agent.Title != "父任务" || e.Agent.Model != "gpt-6-astra" || e.Agent.CWD != "/work" || e.Agent.ShortID != "c6dfda0" {
					t.Fatalf("incomplete parent: %+v", e.Agent)
				}
			}
		})
	}
}

// 查询失败必须可见，不能继续发送只有 ID 的正常通知。
func TestSubagentParentLookupFailure(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"rpc_error", `{"error":{"code":-32603,"message":"lookup unavailable"}}`, 200},
		{"tool_error", `{"result":{"isError":true,"content":[{"type":"text","text":"agent not found"}]}}`, 200},
		{"missing_snapshot", `{"result":{"structuredContent":{}}}`, 200},
		{"wrong_id", `{"result":{"structuredContent":{"snapshot":{"id":"wrong","title":"Wrong parent"}}}}`, 200},
		{"malformed", `{`, 200},
		{"http_error", `unavailable`, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, "data: %s\n\n", tc.body)
			}))
			defer server.Close()
			n := &mockNotifier{}
			w := testWatcher(n)
			defer w.cancel()
			w.mcpURL = server.URL
			w.setupSubagentTracking()
			w.subagentTracker.HandleSubagentUpdate(makeUpdatePayload("parent", "child", "Standards", "codex", "running"))
			w.subagentTracker.HandleSubagentUpdate(makeUpdatePayload("parent", "child", "Standards", "codex", "completed"))
			if len(n.events) != 0 {
				t.Fatalf("lookup failure sent %d incomplete notifications", len(n.events))
			}
		})
	}
}

func TestParentRuntimeModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `data: {"result":{"structuredContent":{"snapshot":{"id":"abc","title":"父任务","model":"configured","runtimeInfo":{"model":"actual"},"cwd":"/work"}}}}`+"\n\n")
	}))
	defer server.Close()
	w := testWatcher(&mockNotifier{})
	defer w.cancel()
	w.mcpURL = server.URL
	a, err := w.fetchParentAgent("abc")
	if err != nil {
		t.Fatal(err)
	}
	if a.Model != "actual" || a.ShortID != "abc" {
		t.Fatalf("incorrect snapshot projection: %+v", a)
	}
}
