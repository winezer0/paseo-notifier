package agentwatcher

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// toolOutputDir OpenCode tool-output 目录的相对路径（相对于 home 下的 .local/share/opencode/）
const toolOutputRel = ".local\\share\\opencode\\tool-output"

// openCodeHomeEnv 可覆盖 OpenCode 数据目录的环境变量
const openCodeHomeEnv = "OPENCODE_DATA_HOME"

var (
	// toolTaskIDPattern 匹配文件中的 "Task ID: bg_xxx"
	toolTaskIDPattern = regexp.MustCompile(`Task ID: (bg_\w+)`)
	// toolDescPattern 匹配 "Description: xxx"
	toolDescPattern = regexp.MustCompile(`Description: (.+)`)
	// toolDurationPattern 匹配 "Duration: xxx"
	toolDurationPattern = regexp.MustCompile(`Duration: (.+)`)
	// toolSessionPattern 匹配 "Session ID: xxx"
	toolSessionPattern = regexp.MustCompile(`Session ID: (\S+)`)
)

// ToolOutputWatcher 监控 OpenCode tool-output 目录，检测新完成的子任务。
type ToolOutputWatcher struct {
	dir  string          // 监控目录路径
	seen map[string]bool // 已处理过的文件名
	mu   sync.Mutex
}

// NewToolOutputWatcher 创建 tool-output 监控器
func NewToolOutputWatcher() *ToolOutputWatcher {
	return &ToolOutputWatcher{
		dir:  detectToolOutputDir(),
		seen: make(map[string]bool),
	}
}

// detectToolOutputDir 检测 tool-output 目录路径
func detectToolOutputDir() string {
	// 1. 环境变量覆盖
	if env := os.Getenv(openCodeHomeEnv); env != "" {
		candidate := filepath.Join(env, "tool-output")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	// 2. 标准路径: ~/.local/share/opencode/tool-output
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := filepath.Join(home, toolOutputRel)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	return ""
}

// Dir 返回监控目录路径
func (w *ToolOutputWatcher) Dir() string {
	return w.dir
}

// Poll 扫描目录中的新文件，解析 Task Result 并返回已完成子任务列表
func (w *ToolOutputWatcher) Poll() []SubagentInfo {
	if w.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var results []SubagentInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if w.seen[name] {
			continue
		}
		w.seen[name] = true

		info := w.parseFile(filepath.Join(w.dir, name))
		if info == nil {
			continue
		}
		results = append(results, *info)
	}
	return results
}

// parseFile 解析单个 tool-output 文件，提取 Task Result 信息
func (w *ToolOutputWatcher) parseFile(path string) *SubagentInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	// 必须是 Task Result 文件
	if !strings.HasPrefix(content, "Task Result") {
		return nil
	}

	m := toolTaskIDPattern.FindStringSubmatch(content)
	if len(m) < 2 {
		return nil
	}

	info := &SubagentInfo{
		Kind:   SubagentOpenCode,
		ID:     m[1],
		Status: "completed",
	}

	if m := toolDescPattern.FindStringSubmatch(content); len(m) >= 2 {
		info.Description = m[1]
	}
	if m := toolDurationPattern.FindStringSubmatch(content); len(m) >= 2 {
		info.Duration = m[1]
	}
	if m := toolSessionPattern.FindStringSubmatch(content); len(m) >= 2 {
		info.SessionID = m[1]
	}

	// 从文件名推测 parent session（文件名格式 tool_<session_hash><random>）
	// 但无法精确对应 agent ID，留空由回调方补充
	return info
}

// Reset 清空已处理文件记录
func (w *ToolOutputWatcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen = make(map[string]bool)
}
