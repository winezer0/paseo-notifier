package agentwatcher

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
)

// completedRetention 已完成子任务保留时间，超过此时间的 completed 条目自动清理
const completedRetention = 10 * time.Minute

// bgTaskIDPattern 匹配活动记录中的 "Task ID: bg_xxx"
var bgTaskIDPattern = regexp.MustCompile(`Task ID: (bg_\w+)`)

// bgTaskDescPattern 匹配 "Description: ..." 行
var bgTaskDescPattern = regexp.MustCompile(`Description: (.+)`)

// bgTaskDurationPattern 匹配 "Duration: ..." 行
var bgTaskDurationPattern = regexp.MustCompile(`Duration: (.+)`)

// bgTaskSessionPattern 匹配 "Session ID: ..." 行
var bgTaskSessionPattern = regexp.MustCompile(`Session ID: (\S+)`)

// launchAgentPattern 匹配后台任务启动记录 "[agent_type] description"。
// agent_type 全小写（explore、general、librarian 等），区别于首字母大写的工具调用如 [Read]、[Thought]。
var launchAgentPattern = regexp.MustCompile(`^\[([a-z][a-z0-9_-]+)\] (.+)`)

// knownAgentTypes OpenCode 后台任务的有效 agent_type 列表
var knownAgentTypes = map[string]bool{
	"explore":            true,
	"general":            true,
	"librarian":          true,
	"oracle":             true,
	"metis":              true,
	"momus":              true,
	"sisyphus-junior":    true,
	"multimodal-looker":  true,
	"visual-engineering": true,
	"artistry":           true,
	"ultrabrain":         true,
	"deep":               true,
	"quick":              true,
	"unspecified-low":    true,
	"unspecified-high":   true,
	"writing":            true,
}

// SubagentTracker 跟踪 agent 的子任务/子 agent 进度。
// 通过解析父 agent 的活动记录，提取并追踪 bg_xxx 子任务的状态变化。
type SubagentTracker struct {
	mu               sync.Mutex
	subagents        map[string]*SubagentInfo // key = bg_xxx 或 agentId
	baselinedParents map[string]bool          // 已完成基线的 parent ID
}

// NewSubagentTracker 创建子任务追踪器
func NewSubagentTracker() *SubagentTracker {
	return &SubagentTracker{
		subagents:        make(map[string]*SubagentInfo),
		baselinedParents: make(map[string]bool),
	}
}

// DetectChanges 解析父 agent 的活动记录，检测子任务状态变化。
// parentID 为父 agent ID，entries 为活动记录列表。
// allowNew=false 时只追踪已有子任务的状态变化（completed/消失），不新增；用于非 running 的 agent 避免历史脏数据。
// 返回新增或状态有变化的子任务列表。无变化时返回 nil。
func (t *SubagentTracker) DetectChanges(parentID string, entries []ActivityEntry, allowNew bool) []SubagentInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 首次遇到此 parent：基线建立阶段，存储所有条目但不返回 changes（completed 除外）
	isBaseline := !t.baselinedParents[parentID]

	var changes []SubagentInfo
	seen := make(map[string]bool)

	for _, entry := range entries {
		info := parseSubagentFromActivity(entry)
		if info == nil {
			continue
		}
		info.ParentID = parentID
		seen[info.ID] = true

		existing, exists := t.subagents[info.ID]
		if !exists {
			// 新增子任务
			t.subagents[info.ID] = info
			if info.Status == "completed" {
				info.CompletedAt = time.Now()
			}
			// 基线阶段：running agent 仍然报告，idle/finished 静默
			if isBaseline && !allowNew && info.Status != "completed" {
				continue
			}
			changes = append(changes, *info)
			logging.Debugf("subagent new parent=%s id=%s status=%s", parentID, info.ID, info.Status)

			// 如果是 completed 条目且有描述，找到匹配的 launch_ 条目也标记完成
			if info.Status == "completed" && info.Description != "" {
				launchID := fmt.Sprintf("launch_%s", info.Description)
				if len(launchID) > 64 {
					launchID = launchID[:64]
				}
				if launch, ok := t.subagents[launchID]; ok && launch.Status != "completed" {
					launch.Status = "completed"
					launch.CompletedAt = time.Now()
					if info.Duration != "" {
						launch.Duration = info.Duration
					}
					changes = append(changes, *launch)
					logging.Debugf("subagent launch matched completion parent=%s id=%s", parentID, launchID)
				}
			}
		} else if existing.Status != info.Status {
			// 状态变化：只允许 running→completed，禁止 completed→running 降级
			if existing.Status == "completed" {
				continue
			}
			existing.Status = info.Status
			if info.Duration != "" {
				existing.Duration = info.Duration
			}
			if existing.Status == "completed" {
				existing.CompletedAt = time.Now()
			}
			changes = append(changes, *existing)
			logging.Debugf("subagent status parent=%s id=%s -> %s", parentID, info.ID, info.Status)
		}
		// 已存在且状态相同，跳过
	}

	// 清理当前父 agent 下不再出现的子任务（标记为 completed）
	// 同时处理 bg_xxx 和 launch_ 前缀的条目
	for id, sa := range t.subagents {
		if sa.ParentID != parentID || seen[id] || sa.Status == "completed" {
			continue
		}
		if strings.HasPrefix(id, "bg_") || strings.HasPrefix(id, "launch_") {
			sa.Status = "completed"
			sa.CompletedAt = time.Now()
			changes = append(changes, *sa)
			logging.Debugf("subagent completed (no longer active) parentId=%s subId=%s",
				parentID, id)
		}
	}

	// 清理超过保留时间的已完成条目
	t.cleanupCompleted()

	// 标记此 parent 已完成基线
	if isBaseline {
		t.baselinedParents[parentID] = true
	}

	if len(changes) == 0 {
		return nil
	}
	return changes
}

// InjectCompleted 从 tool-output 注入一个已完成子任务。
// 如果 tracker 中已有同 ID 条目则更新状态；否则新增。
// 返回 changes（可能是 0/1 条）。
func (t *SubagentTracker) InjectCompleted(info SubagentInfo) []SubagentInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	info.Status = "completed"
	info.CompletedAt = time.Now()

	var changes []SubagentInfo
	existing, exists := t.subagents[info.ID]
	if !exists {
		t.subagents[info.ID] = &info
		// 按描述匹配 launch_ 条目
		if info.Description != "" {
			launchID := fmt.Sprintf("launch_%s", info.Description)
			if len(launchID) > 64 {
				launchID = launchID[:64]
			}
			if launch, ok := t.subagents[launchID]; ok && launch.Status != "completed" {
				launch.Status = "completed"
				launch.CompletedAt = time.Now()
				if info.Duration != "" {
					launch.Duration = info.Duration
				}
				changes = append(changes, *launch)
			}
		}
		// 新增条目本身也返回
		info.ParentID = ""
		changes = append(changes, info)
	} else if existing.Status != "completed" {
		existing.Status = "completed"
		existing.CompletedAt = time.Now()
		if info.Duration != "" {
			existing.Duration = info.Duration
		}
		changes = append(changes, *existing)
	}

	if len(changes) == 0 {
		return nil
	}
	return changes
}

// cleanupCompleted 清理超过 completedRetention 的已完成条目
func (t *SubagentTracker) cleanupCompleted() {
	cutoff := time.Now().Add(-completedRetention)
	for id, sa := range t.subagents {
		if sa.Status == "completed" && !sa.CompletedAt.IsZero() && sa.CompletedAt.Before(cutoff) {
			delete(t.subagents, id)
		}
	}
}

// GetAll 返回当前追踪的所有子任务快照
func (t *SubagentTracker) GetAll() []SubagentInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]SubagentInfo, 0, len(t.subagents))
	for _, sa := range t.subagents {
		result = append(result, *sa)
	}
	return result
}

// GetByParent 返回指定父 agent 的所有子任务
func (t *SubagentTracker) GetByParent(parentID string) []SubagentInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []SubagentInfo
	for _, sa := range t.subagents {
		if sa.ParentID == parentID {
			result = append(result, *sa)
		}
	}
	return result
}

// Reset 清空所有追踪状态
func (t *SubagentTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subagents = make(map[string]*SubagentInfo)
}

// parseSubagentFromActivity 从单条活动记录中提取 OpenCode 后台子任务信息（bg_xxx）。
//
// 解析优先级：
//  0. entry.Type 为已知 agent_type → running（从文本摘要解析的 "[type] desc"）
//  1. "[agent_type] description" → running（启动阶段，agent_type 全小写）
//  2. "subagent/parallel" + "bg_" → running（显式启动记录）
//  3. "Task Result" + "Task ID: bg_" → completed（有输出结果）
//  4. 裸 "Task ID: bg_xxx" → running（默认）
func parseSubagentFromActivity(entry ActivityEntry) *SubagentInfo {
	summary := entry.Summary
	if summary == "" {
		return nil
	}

	// 0. 从文本摘要解析：entry.Type 为已知 agent_type → running
	// 当活动记录从文本摘要解析时，parseActivityLine 已提取 Type，summary 不含 [Type] 前缀
	if knownAgentTypes[entry.Type] && !strings.HasPrefix(summary, "[") {
		maxDesc := summary
		if len(maxDesc) > 60 {
			maxDesc = maxDesc[:60]
		}
		return &SubagentInfo{
			Kind:        SubagentOpenCode,
			ID:          fmt.Sprintf("launch_%s", maxDesc),
			Status:      "running",
			Description: summary,
		}
	}

	// 1. 启动阶段：匹配 "[agent_type] description"（agent_type 全小写，区别于工具调用）
	if m := launchAgentPattern.FindStringSubmatch(summary); len(m) >= 3 {
		if knownAgentTypes[m[1]] {
			desc := m[2]
			// 如果 description 本身也以 [ 开头（如 "[Read] file.go"），说明不是子任务而是工具调用
			if !strings.HasPrefix(desc, "[") {
				maxDesc := desc
				if len(maxDesc) > 60 {
					maxDesc = maxDesc[:60]
				}
				return &SubagentInfo{
					Kind:        SubagentOpenCode,
					ID:          fmt.Sprintf("launch_%s", maxDesc),
					Status:      "running",
					Description: desc,
				}
			}
		}
	}

	// 1. 启动阶段：包含 "subagent"/"parallel" 关键词且引用了 bg_xxx → running
	if (strings.Contains(summary, "subagent") || strings.Contains(summary, "parallel")) &&
		strings.Contains(summary, "bg_") {
		if m := bgTaskIDPattern.FindStringSubmatch(summary); len(m) >= 2 {
			return &SubagentInfo{
				Kind:   SubagentOpenCode,
				ID:     m[1],
				Status: "running",
			}
		}
	}

	// 2. 完成阶段：包含 "Task Result" → completed，附带详细信息
	if strings.Contains(summary, "Task Result") {
		if m := bgTaskIDPattern.FindStringSubmatch(summary); len(m) >= 2 {
			info := &SubagentInfo{
				Kind:   SubagentOpenCode,
				ID:     m[1],
				Status: "completed",
			}
			if m := bgTaskDescPattern.FindStringSubmatch(summary); len(m) >= 2 {
				info.Description = m[1]
			}
			if m := bgTaskDurationPattern.FindStringSubmatch(summary); len(m) >= 2 {
				info.Duration = m[1]
			}
			if m := bgTaskSessionPattern.FindStringSubmatch(summary); len(m) >= 2 {
				info.SessionID = m[1]
			}
			return info
		}
	}

	// 3. 裸 "Task ID: bg_xxx" → running
	if m := bgTaskIDPattern.FindStringSubmatch(summary); len(m) >= 2 {
		return &SubagentInfo{
			Kind:   SubagentOpenCode,
			ID:     m[1],
			Status: "running",
		}
	}

	return nil
}
