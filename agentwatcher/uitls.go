package agentwatcher

import "strings"

// normalizeDaemonURL 兼容旧配置：若传入的 URL 以 /mcp/agents 结尾，自动剥离后缀返回基础地址
func normalizeDaemonURL(raw string) string {
	if strings.HasSuffix(raw, "/mcp/agents") {
		return raw[:len(raw)-len("/mcp/agents")]
	}
	return raw
}
