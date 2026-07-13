package agentwatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// fetchAgents 调用 list_agents MCP 工具获取 Agent 列表
func (w *Watcher) fetchAgents() ([]AgentStatus, error) {
	resp, err := w.callMCP("list_agents", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("mcp call failed: %w", err)
	}

	var result listAgentsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse agents response failed: %w", err)
	}

	// debug: 打印 list_agents Content 长度，排查 archivedAt 缺失原因
	for i, c := range result.Result.Content {
		if c.Text != "" {
			logging.Debugf("list_agents content[%d] type=%s len=%d", i, c.Type, len(c.Text))
		}
	}

	return result.Result.StructuredContent.Agents, nil
}

// fetchPendingPermissions 调用 list_pending_permissions MCP 工具获取待处理权限请求
func (w *Watcher) fetchPendingPermissions() ([]PermissionRequest, error) {
	resp, err := w.callMCP("list_pending_permissions", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("mcp call failed: %w", err)
	}

	var result listPermissionsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse permissions response failed: %w", err)
	}

	return result.Result.StructuredContent.Permissions, nil
}

// agentActivityInner Content[].Text 内层 JSON 结构
type agentActivityInner struct {
	Content string `json:"content"` // 活动文本摘要
}

// getAgentActivity 调用 get_agent_activity MCP 工具，返回 Agent 的活动记录列表
// 调用失败时返回 nil
func (w *Watcher) getAgentActivity(agentID string) []ActivityEntry {
	resp, err := w.callMCP("get_agent_activity", map[string]interface{}{
		"agentId": agentID,
		"limit":   100, // 只取最近 100 条，减少数据量
	})
	if err != nil {
		logging.Errorf("get_agent_activity failed agentId=%s err=%v", agentID, err)
		return nil
	}

	var result agentActivityResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		logging.Warnf("parse agent activity response failed agentId=%s err=%v", agentID, err)
		return nil
	}

	// 优先使用 StructuredContent.Entries
	if result.Result.StructuredContent != nil && len(result.Result.StructuredContent.Entries) > 0 {
		return result.Result.StructuredContent.Entries
	}

	// 备选：从 Content[].Text JSON 中解析
	// Content[0].Text 格式: {"agentId":"...","updateCount":...,"content":"Showing X of Y activities\n\n[entry]\n..."}
	for _, c := range result.Result.Content {
		if len(c.Text) == 0 {
			continue
		}
		var inner agentActivityInner
		if err := json.Unmarshal([]byte(c.Text), &inner); err != nil {
			continue
		}
		entries := parseActivityTextDump(inner.Content)
		if len(entries) > 0 {
			return entries
		}
	}
	return nil
}

// parseActivityTextDump 解析 "Showing X of Y activities\n\n[type] content\n..." 格式的文本为 ActivityEntry 列表
func parseActivityTextDump(text string) []ActivityEntry {
	lines := strings.Split(text, "\n")
	var entries []ActivityEntry
	started := false
	buf := "" // 多行累计缓冲
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !started {
			if strings.HasPrefix(trimmed, "Showing ") && strings.Contains(trimmed, " activities") {
				started = true
			}
			continue
		}
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			// 新活动行，先 flush 缓冲
			if buf != "" {
				entries = append(entries, ActivityEntry{
					Summary: buf,
				})
				buf = ""
			}
			entryType, summary := parseActivityLine(trimmed)
			if len(summary) > 50 {
				summary = summary[:50] + "..."
			}
			entries = append(entries, ActivityEntry{
				Type:    entryType,
				Summary: summary,
			})
		} else {
			// 无 bracket 前缀的行：可能是 Task Result 的多行内容
			// 仅当有累计缓冲或包含关键信息时才保留
			if buf != "" || strings.Contains(trimmed, "Task ID:") ||
				strings.Contains(trimmed, "Task Result") ||
				strings.Contains(trimmed, "Duration:") ||
				strings.Contains(trimmed, "Session ID:") {
				if buf != "" {
					buf += "\n"
				}
				buf += trimmed
			}
		}
	}
	// flush 剩余缓冲
	if buf != "" {
		entries = append(entries, ActivityEntry{
			Summary: buf,
		})
	}
	return entries
}

// parseActivityLine 从 "[Type] content" 格式的行中提取 Type 和 content
func parseActivityLine(line string) (entryType, summary string) {
	if len(line) < 2 || line[0] != '[' {
		return "", line
	}
	if idx := strings.Index(line, "] "); idx > 0 {
		entryType = line[1:idx]
		summary = line[idx+2:]
	} else if idx := strings.Index(line, "]"); idx > 0 {
		// "[Type]" 无内容
		entryType = line[1:idx]
		summary = ""
	} else {
		summary = line
	}
	return
}

// getAgentStatus 调用 get_agent_status MCP 工具，返回 Agent 的最新状态文本摘要
func (w *Watcher) getAgentStatus(agentID string) string {
	resp, err := w.callMCPWithTimeout("get_agent_status", map[string]interface{}{
		"agentId": agentID,
	}, 10*time.Second)
	if err != nil {
		logging.Warnf("get_agent_status failed agentId=%s err=%v", agentID, err)
		return ""
	}

	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		logging.Warnf("parse agent status failed agentId=%s err=%v", agentID, err)
		return ""
	}
	if len(result.Result.Content) > 0 {
		return result.Result.Content[0].Text
	}
	return ""
}

// lastActivityTime 返回活动记录中最后一条的时间戳，没有记录时返回 nil
func (w *Watcher) lastActivityTime(entries []ActivityEntry) *time.Time {
	var latest *time.Time
	for _, entry := range entries {
		if entry.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		if latest == nil || t.After(*latest) {
			latest = &t
		}
	}
	return latest
}

// createAgent 调用 create_agent MCP 工具创建一个 Agent，返回 Agent ID
// 创建 Agent 可能耗时较长，使用独立的 60s 超时
func (w *Watcher) createAgent(cwd, title, provider, initialPrompt string) (string, error) {
	resp, err := w.callMCPWithTimeout("create_agent", map[string]interface{}{
		"cwd":           cwd,
		"title":         title,
		"provider":      provider,
		"initialPrompt": initialPrompt,
	}, 60*time.Second)
	if err != nil {
		return "", fmt.Errorf("create_agent failed: %w", err)
	}

	var result struct {
		Result struct {
			StructuredContent *struct {
				AgentID string `json:"agentId"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse create_agent response failed: %w", err)
	}
	if result.Result.StructuredContent == nil {
		return "", fmt.Errorf("create_agent response missing structuredContent: %s", string(resp))
	}

	logging.Infof("agent created agentId=%s title=%s", result.Result.StructuredContent.AgentID, title)
	return result.Result.StructuredContent.AgentID, nil
}

// sendAgentPrompt 调用 send_agent_prompt MCP 工具向指定 Agent 发送提示
func (w *Watcher) sendAgentPrompt(agentID, prompt string) error {
	_, err := w.callMCPWithTimeout("send_agent_prompt", map[string]interface{}{
		"agentId": agentID,
		"prompt":  prompt,
	}, 60*time.Second)
	if err != nil {
		return fmt.Errorf("send_agent_prompt failed: %w", err)
	}
	logging.Infof("agent prompt sent agentId=%s", agentID)
	return nil
}

// archiveAgent 调用 archive_agent MCP 工具软删除指定 Agent
func (w *Watcher) archiveAgent(agentID string) error {
	_, err := w.callMCPWithTimeout("archive_agent", map[string]interface{}{
		"agentId": agentID,
	}, 30*time.Second)
	if err != nil {
		return fmt.Errorf("archive_agent failed: %w", err)
	}
	logging.Infof("agent archived agentId=%s", agentID)
	return nil
}

// cancelAgent 调用 cancel_agent MCP 工具取消指定 Agent 的当前任务
func (w *Watcher) cancelAgent(agentID string) error {
	_, err := w.callMCPWithTimeout("cancel_agent", map[string]interface{}{
		"agentId": agentID,
	}, 30*time.Second)
	if err != nil {
		return fmt.Errorf("cancel_agent failed: %w", err)
	}
	logging.Infof("agent cancelled agentId=%s", agentID)
	return nil
}

// continueAgent 调用 send_agent_prompt MCP 工具发送继续任务提示
func (w *Watcher) continueAgent(agentID, prompt string) error {
	if prompt == "" {
		return fmt.Errorf("continue prompt not set")
	}
	_, err := w.callMCPWithTimeout("send_agent_prompt", map[string]interface{}{
		"agentId": agentID,
		"prompt":  prompt,
	}, 120*time.Second)
	if err != nil {
		return fmt.Errorf("send_agent_prompt failed: %w", err)
	}
	logging.Infof("continue prompt sent agentId=%s", agentID)
	return nil
}

// restartAgent 尝试恢复卡死的 Agent，保留上下文
// 只发继续提示，不取消执行，保护用户对话上下文不被损坏
func (w *Watcher) restartAgent(agentID string) error {
	if err := w.continueAgent(agentID, w.stuckContinuePrompt); err != nil {
		return fmt.Errorf("continue prompt failed: %w", err)
	}
	return nil
}

// callMCP 发送 MCP JSON-RPC 请求并返回响应体中的 data: 行内容
// 使用 watcher 默认的 HTTP 客户端超时
func (w *Watcher) callMCP(method string, params interface{}) ([]byte, error) {
	return w.callMCPWithTimeout(method, params, 0)
}

// callMCPWithTimeout 发送 MCP JSON-RPC 请求，支持自定义超时
// timeout=0 时使用 watcher 默认的 httpClient 超时
func (w *Watcher) callMCPWithTimeout(method string, params interface{}, timeout time.Duration) ([]byte, error) {
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      w.nextID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      method,
			"arguments": params,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 如果指定了超时，创建独立的 context 避免被 watcher 生命周期影响；否则使用 watcher 的 context
	reqCtx := w.ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, "POST", w.mcpURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// 如果使用自定义超时，创建独立的 httpClient 绕过默认的 10s 超时
	httpClient := w.httpClient
	if timeout > 0 {
		httpClient = &http.Client{
			Timeout:   timeout + 5*time.Second,
			Transport: w.httpClient.Transport,
		}
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSSEJSON(respBody)
}

// parseSSEJSON 从 SSE 响应中提取 data: 行的 JSON 内容
func parseSSEJSON(data []byte) ([]byte, error) {
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data: ")) {
			return bytes.TrimPrefix(trimmed, []byte("data: ")), nil
		}
	}
	return nil, fmt.Errorf("no data line found in SSE response: %s", string(data))
}

// ptrTimeEqual 比较两个 *string 时间戳是否相等
func ptrTimeEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
