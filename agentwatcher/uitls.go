package agentwatcher

import "strings"

// normalizeDaemonURL 兼容旧配置：若传入的 URL 以 /mcp/agents 结尾，自动剥离后缀返回基础地址
func normalizeDaemonURL(raw string) string {
	if strings.HasSuffix(raw, "/mcp/agents") {
		return raw[:len(raw)-len("/mcp/agents")]
	}
	return raw
}

// subagentDoneDefaultPrompt 子任务全部完成后发送给主 agent 的默认继续提示
func subagentDoneDefaultPrompt() string {
	return "检测到子任务可能都已经完成，请检查子任务状态，并继续完成主任务。"
}
