// paseo-mcp — 手动诊断工具，输出 Agent 的 4 类关键状态：
//
//   - 🟡 正在运行的任务
//   - ✅ 任务完成（finished）
//   - ❌ 任务错误（error）
//   - 📝 需要交互（permission request）
//
// 用法:
//
//	paseo-mcp                         # 默认持续监控
//	paseo-mcp --once                  # 单次查询
//	paseo-mcp --interval 10s          # 自定义轮询间隔
//	paseo-mcp --url http://..."       # 指定守护进程地址
//
// 守护进程地址默认从 paseo-notifier.yaml 读取，未找到则使用
// http://127.0.0.1:6767/mcp/agents。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
)

type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type agentInfo struct {
	ID                 string
	Title              string
	Status             string
	AttentionReason    *string
	AttentionTimestamp *string
}

func main() {
	once := flag.Bool("once", false, "single query mode (default: continuous)")
	interval := flag.Duration("interval", 5*time.Second, "polling interval")
	daemonURL := flag.String("url", "", "daemon MCP URL")
	flag.Parse()

	url := resolveDaemonURL(*daemonURL)

	if *once {
		if err := queryOnce(url); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := watchLoop(url, *interval); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func resolveDaemonURL(custom string) string {
	if custom != "" {
		return custom
	}
	cfg, err := config.Load("")
	if err == nil && cfg.Monitor.DaemonURL != "" {
		return cfg.Monitor.DaemonURL
	}
	return "http://127.0.0.1:6767/mcp/agents"
}

func watchLoop(url string, interval time.Duration) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("Monitoring %s (every %s, Ctrl+C to stop)\n\n", url, interval)

	// collect state for change detection
	prev := make(map[string]*agentInfo)

	queryAndShow(url, prev)

	for {
		select {
		case <-ticker.C:
			clearConsole()
			fmt.Printf("Monitoring %s (every %s, Ctrl+C to stop)\n\n", url, interval)
			queryAndShow(url, prev)
		case <-sigCh:
			fmt.Println("\nstopped")
			return nil
		}
	}
}

func clearConsole() {
	fmt.Print("\033[H\033[2J")
}

func queryOnce(url string) error {
	prev := make(map[string]*agentInfo)
	agents, err := fetchAgents(url)
	if err != nil {
		return fmt.Errorf("fetch agents: %w", err)
	}
	permissions, _ := fetchPermissions(url)
	showStatus(agents, permissions, prev)
	return nil
}

func queryAndShow(url string, prev map[string]*agentInfo) {
	agents, err := fetchAgents(url)
	if err != nil {
		fmt.Printf("\033[31m[MCP ERROR] %v\033[0m\n\n", err)
		return
	}

	permissions, err := fetchPermissions(url)
	if err != nil {
		permissions = nil
	}

	showStatus(agents, permissions, prev)
}

func fetchAgents(url string) ([]agentwatcher.AgentStatus, error) {
	data, err := callMCP(url, "list_agents", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var raw struct {
		StructuredContent struct {
			Agents []agentwatcher.AgentStatus `json:"agents"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return raw.StructuredContent.Agents, nil
}

func fetchPermissions(url string) ([]agentwatcher.PermissionRequest, error) {
	data, err := callMCP(url, "list_pending_permissions", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var raw struct {
		StructuredContent struct {
			Permissions []agentwatcher.PermissionRequest `json:"permissions"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return raw.StructuredContent.Permissions, nil
}

func callMCP(url, method string, params interface{}) (json.RawMessage, error) {
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      method,
			"arguments": params,
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBody))
	}
	return parseSSE(respBody)
}

func parseSSE(data []byte) (json.RawMessage, error) {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			result := strings.TrimPrefix(line, "data: ")
			var rpc struct {
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(result), &rpc); err != nil {
				return nil, fmt.Errorf("parse response JSON: %w", err)
			}
			if rpc.Error != nil {
				return nil, fmt.Errorf("RPC error %d: %s", rpc.Error.Code, rpc.Error.Message)
			}
			return rpc.Result, nil
		}
	}
	return nil, fmt.Errorf("no data line found in response")
}

func showStatus(agents []agentwatcher.AgentStatus, permissions []agentwatcher.PermissionRequest, prev map[string]*agentInfo) {
	var running []agentwatcher.AgentStatus
	var finished []agentwatcher.AgentStatus
	var errored []agentwatcher.AgentStatus

	for _, a := range agents {
		// track for change detection
		prev[a.ID] = &agentInfo{
			ID:     a.ID,
			Title:  a.Title,
			Status: a.Status,
			AttentionReason:    a.AttentionReason,
			AttentionTimestamp: a.AttentionTimestamp,
		}

		switch {
		case a.Status == "running":
			running = append(running, a)
		case a.RequiresAttention && a.AttentionReason != nil && *a.AttentionReason == "finished":
			finished = append(finished, a)
		case a.RequiresAttention && a.AttentionReason != nil && *a.AttentionReason == "error":
			errored = append(errored, a)
		}
	}

	// 1) running
	if len(running) > 0 {
		fmt.Printf("\033[33m🟡 正在运行 (%d)\033[0m\n", len(running))
		for _, a := range running {
			fmt.Printf("  %s  %s\n", a.ShortID, a.Title)
		}
		fmt.Println()
	}

	// 2) finished
	if len(finished) > 0 {
		fmt.Printf("\033[32m✅ 已完成 (%d)\033[0m\n", len(finished))
		for _, a := range finished {
			ts := ""
			if a.AttentionTimestamp != nil && *a.AttentionTimestamp != "" {
				ts = " @" + *a.AttentionTimestamp
			}
			fmt.Printf("  %s  %s%s\n", a.ShortID, a.Title, ts)
		}
		fmt.Println()
	}

	// 3) error
	if len(errored) > 0 {
		fmt.Printf("\033[31m❌ 出错 (%d)\033[0m\n", len(errored))
		for _, a := range errored {
			ts := ""
			if a.AttentionTimestamp != nil && *a.AttentionTimestamp != "" {
				ts = " @" + *a.AttentionTimestamp
			}
			fmt.Printf("  %s  %s%s\n", a.ShortID, a.Title, ts)
		}
		fmt.Println()
	}

	// 4) permission requests
	var pending []agentwatcher.PermissionRequest
	for _, p := range permissions {
		if p.Status == "running" {
			pending = append(pending, p)
		}
	}
	if len(pending) > 0 {
		fmt.Printf("\033[36m📝 需要交互 (%d)\033[0m\n", len(pending))
		for _, p := range pending {
			desc := p.Request.Title
			if p.Request.Description != "" {
				desc += " — " + p.Request.Description
			}
			fmt.Printf("  [%s] %s  (agent: %s)\n", p.Request.Kind, desc, p.AgentID)
		}
		fmt.Println()
	}

	// nothing at all
	if len(running)+len(finished)+len(errored)+len(pending) == 0 {
		fmt.Println("暂无任务状态")
	}
}
