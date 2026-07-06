# Paseo MCP

Paseo 可以将以下 MCP 工具注入到每个新启动的 agent 中。在宿主设置中开启 **Inject Paseo tools**，或将 `daemon.mcp.injectIntoAgents` 设为 `true` 即可。

MCP server 本身由 `daemon.mcp.enabled` 控制。已存在的 agent 可能需要重新加载才能生效。

## 工具一览

### Agents（11 个）

| 工具 | 功能 |
|---|---|
| `create_agent` | 创建一个绑定到工作目录的 agent，可指定初始配置或新的 git worktree |
| `wait_for_agent` | 阻塞等待，直到 agent 请求权限或完成当前 run |
| `send_agent_prompt` | 向运行中的 agent 发送任务 |
| `get_agent_status` | 返回 agent 最新状态快照 |
| `list_agents` | 列出最近 agent 的紧凑元数据 |
| `cancel_agent` | 中止 agent 的当前 run，但保留 agent 存活 |
| `archive_agent` | 软删除 agent，并将其从活动列表中移除 |
| `kill_agent` | 永久终止 agent 会话 |
| `update_agent` | 更新 agent 名称、标签，或运行时配置（mode / model / thinking / features） |
| `get_agent_activity` | 返回 agent 最近时间线的整理摘要 |
| `set_agent_mode` | 切换 agent 的会话模式 |

### Terminals（5 个）

| 工具 | 功能 |
|---|---|
| `list_terminals` | 列出指定工作目录（或全部工作目录）的终端会话 |
| `create_terminal` | 为工作目录创建终端会话 |
| `kill_terminal` | 终止终端会话 |
| `capture_terminal` | 捕获终端会话的纯文本输出 |
| `send_terminal_keys` | 向终端会话发送文本或特殊按键 |

### Schedules（6 个）

| 工具 | 功能 |
|---|---|
| `create_schedule` | 创建周期性 schedule，可作用于现有 agent 或新建 agent |
| `list_schedules` | 列出 daemon 管理的 schedule |
| `inspect_schedule` | 查看 schedule 详情及运行历史 |
| `pause_schedule` | 暂停运行中的 schedule |
| `resume_schedule` | 恢复已暂停的 schedule |
| `delete_schedule` | 永久删除 schedule |

### Providers（3 个）

| 工具 | 功能 |
|---|---|
| `list_providers` | 列出已配置的 agent provider、可用性及模式 |
| `list_models` | 列出指定 provider 的模型 |
| `inspect_provider` | 查看 provider 的紧凑能力描述，并草拟 feature 配置 |

### Worktrees（3 个）

| 工具 | 功能 |
|---|---|
| `list_worktrees` | 列出 Paseo 管理的 git worktree |
| `create_worktree` | 从分支、基础分支或 GitHub PR 创建 Paseo 管理的 git worktree |
| `archive_worktree` | 删除 Paseo 管理的 git worktree |

### Permissions（2 个）

| 工具 | 功能 |
|---|---|
| `list_pending_permissions` | 返回所有 agent 的待处理权限请求 |
| `respond_to_permission` | 批准或拒绝待处理的权限请求 |

### Voice（1 个）

| 工具 | 功能 |
|---|---|
| `speak` | 通过 daemon 管理的语音输出朗读文本，仅在 voice-enabled 会话中可用 |

## 相关配置

| 配置项 | 说明 |
|---|---|
| `daemon.mcp.enabled` | 控制 MCP server 总开关 |
| `daemon.mcp.injectIntoAgents` | 是否将 Paseo 工具自动注入到新 agent |

## 合计

总计 **31 个 MCP 工具**，分布于 **7 个分类**。