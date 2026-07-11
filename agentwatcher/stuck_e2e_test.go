//go:build integration

package agentwatcher

import (
	"os"
	"testing"
	"time"

	"github.com/winezer0/paseo-notifier/config"
)

// TestStuckDetectionE2E 卡死检测端到端测试
//
// 创建 Agent 后不发送额外任务，利用 Agent 处理完初始 prompt 后的空闲状态触发卡死检测。
// 验证 watcher 能正确检测卡死、发送通知、并自动重启 Agent.
//
// 运行方式：
//
//	go test -tags=integration -run TestStuckDetectionE2E -timeout=5m ./agentwatcher/
//
// 环境变量：
//
//	PASEO_DAEMON_URL - 守护进程地址（默认 http://127.0.0.1:6767/mcp/agents）
//	PASEO_TEST_CWD  - Agent 工作目录（默认当前目录）
//	PASEO_TEST_PROVIDER - 供应商/模型（默认 opencode/nemotron-3-ultra-free）
func TestStuckDetectionE2E(t *testing.T) {
	daemonURL := os.Getenv("PASEO_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://127.0.0.1:6767/mcp/agents"
	}

	testCWD := os.Getenv("PASEO_TEST_CWD")
	if testCWD == "" {
		var err error
		testCWD, err = os.Getwd()
		if err != nil {
			t.Fatalf("getwd failed: %v", err)
		}
	}

	provider := os.Getenv("PASEO_TEST_PROVIDER")
	if provider == "" {
		provider = "opencode/nemotron-3-ultra-free"
	}

	notifier := &mockNotifier{}
	w := NewWatcher(config.MonitorConfig{
		DaemonURL:          daemonURL,
		Interval:           "5s",
		StuckDetectTimeout: "10s",
		StuckRestartDelay:  "20s",
		StuckRestartRetry:  5,
	}, notifier, "继续任务", "检测到 Agent 长时间无响应，请检查你的执行状态，从之前的工作继续。如果你不记得之前的任务，请重新询问用户。")

	// 1. 创建测试 Agent
	agentID, err := w.createAgent(testCWD, "stuck-e2e-test", provider, "请等待")
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	t.Logf("test agent created agentId=%s", agentID)
	// 不 cancel 不清档

	// 2. 发送长时间任务：sleep 60 s，使 Agent 进入睡眠状态
	t.Log("sending sleep prompt to agent...")
	if err := w.sendAgentPrompt(agentID, "请执行 sleep 60 秒，等待期间不要做其他操作"); err != nil {
		t.Fatalf("send prompt failed: %v", err)
	}
	t.Log("sleep prompt sent, agent should be stuck for 60s")

	// 3. 等待 Agent 开始执行 sleep（让模型处理完 prompt 开始执行）
	t.Log("waiting for agent to settle (30s)...")
	for i := 0; i < 6; i++ {
		time.Sleep(5 * time.Second)
		agents, err := w.fetchAgents()
		if err == nil {
			for _, a := range agents {
				if a.ID == agentID {
					t.Logf("  agent: status=%s updatedAt=%s reason=%v",
						a.Status, a.UpdatedAt, a.AttentionReason)
					break
				}
			}
		}
	}

	// 3. 设置白名单，只监控测试 Agent，避免影响其他 Agent
	w.SetManagedAgents(agentID)

	// 4. 启动 watcher
	w.Start()
	defer w.Stop()

	// 4. 等待卡死通知（10s）
	t.Log("waiting for stuck notification...")
	stuckDeadline := time.Now().Add(60 * time.Second)
	gotStuckEvent := false
	for time.Now().Before(stuckDeadline) {
		for _, ev := range notifier.events {
			if ev.Type == EventStuck && ev.Agent.ID == agentID {
				gotStuckEvent = true
				break
			}
		}
		if gotStuckEvent {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !gotStuckEvent {
		t.Fatal("stuck event not received within timeout")
	}
	t.Log("stuck event received, agent correctly detected as stuck")

	// 5. 等待自动重启（30s = 3*10s）
	t.Log("waiting for auto-restart...")
	restartDeadline := time.Now().Add(60 * time.Second)
	gotRestart := false
	for time.Now().Before(restartDeadline) {
		agents, err := w.fetchAgents()
		if err == nil {
			for _, a := range agents {
				if a.ID == agentID {
					if a.AttentionReason != nil {
						t.Logf("agent finished after restart: reason=%s", *a.AttentionReason)
						gotRestart = true
					} else if a.Status == "idle" {
						t.Log("agent is idle, restart completed")
						gotRestart = true
					}
					break
				}
			}
		}
		if gotRestart {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if !gotRestart {
		entries := w.getAgentActivity(agentID)
		t.Logf("final activity entries: %d", len(entries))
		for _, entry := range entries {
			t.Logf("  [%s] %s: %s", entry.Timestamp, entry.Type, entry.Summary)
		}
		t.Error("auto-restart not detected within timeout")
	}
}

// TestMCPConnectivity 验证 MCP API 连通性：查询 Agent、创建、取消、归档
// 不依赖模型推理，仅测试 MCP 调用链
func TestMCPConnectivity(t *testing.T) {
	daemonURL := os.Getenv("PASEO_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://127.0.0.1:6767/mcp/agents"
	}

	testCWD := os.Getenv("PASEO_TEST_CWD")
	if testCWD == "" {
		var err error
		testCWD, err = os.Getwd()
		if err != nil {
			t.Fatalf("getwd failed: %v", err)
		}
	}

	provider := os.Getenv("PASEO_TEST_PROVIDER")
	if provider == "" {
		provider = "opencode/nemotron-3-ultra-free"
	}

	notifier := &mockNotifier{}
	w := NewWatcher(config.MonitorConfig{
		DaemonURL: daemonURL,
		Interval:  "5s",
	}, notifier, "继续任务", "检测到 Agent 长时间无响应，请检查你的执行状态，从之前的工作继续。如果你不记得之前的任务，请重新询问用户。")

	// 1. list_agents 连通性验证
	agents, err := w.fetchAgents()
	if err != nil {
		t.Fatalf("fetch agents failed: %v", err)
	}
	t.Logf("list_agents ok: %d agents found", len(agents))

	// 2. create_agent 连通性验证
	agentID, err := w.createAgent(testCWD, "connectivity-test", provider, "请等待")
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	t.Logf("create_agent ok: %s", agentID)

	// 3. cancel_agent 连通性验证
	if err := w.cancelAgent(agentID); err != nil {
		t.Fatalf("cancel agent failed: %v", err)
	}
	t.Log("cancel_agent ok")

	// 4. archive_agent 连通性验证
	if err := w.archiveAgent(agentID); err != nil {
		t.Fatalf("archive agent failed: %v", err)
	}
	t.Log("archive_agent ok")
}


