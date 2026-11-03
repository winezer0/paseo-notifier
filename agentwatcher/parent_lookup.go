package agentwatcher

import (
	"encoding/json"
	"fmt"
)

// fetchParentAgent 按 ID 获取父任务快照，不使用可能截断或过滤的列表。
func (w *Watcher) fetchParentAgent(agentID string) (AgentStatus, error) {
	resp, err := w.callMCP("get_agent_status", map[string]interface{}{"agentId": agentID})
	if err != nil {
		return AgentStatus{}, fmt.Errorf("get_agent_status: %w", err)
	}
	var response struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent struct {
				Snapshot *struct {
					AgentStatus
					RuntimeInfo *struct {
						Model string `json:"model"`
					} `json:"runtimeInfo"`
				} `json:"snapshot"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &response); err != nil {
		return AgentStatus{}, fmt.Errorf("decode get_agent_status: %w", err)
	}
	if response.Error != nil {
		return AgentStatus{}, fmt.Errorf("get_agent_status RPC error code=%d: %s", response.Error.Code, response.Error.Message)
	}
	if response.Result.IsError {
		return AgentStatus{}, fmt.Errorf("get_agent_status tool error: %v", response.Result.Content)
	}
	snapshot := response.Result.StructuredContent.Snapshot
	if snapshot == nil {
		return AgentStatus{}, fmt.Errorf("get_agent_status missing structured snapshot")
	}
	if snapshot.ID != agentID {
		return AgentStatus{}, fmt.Errorf("get_agent_status snapshot ID mismatch: requested=%s received=%s", agentID, snapshot.ID)
	}
	agent := snapshot.AgentStatus
	// 与 Paseo 列表的字段投影保持一致：短 ID 由完整 ID 派生，运行时模型优先。
	agent.ShortID = agent.ID[:min(7, len(agent.ID))]
	if snapshot.RuntimeInfo != nil && snapshot.RuntimeInfo.Model != "" {
		agent.Model = snapshot.RuntimeInfo.Model
	}
	return agent, nil
}
