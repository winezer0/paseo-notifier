# Paseo MCP

Paseo 可以将以下 MCP 工具注入到每个新启动的 agent 中。在宿主设置中开启 **Inject Paseo tools**，或将 `daemon.mcp.injectIntoAgents` 设为 `true` 即可。

MCP server 本身由 `daemon.mcp.enabled` 控制。已存在的 agent 可能需要重新加载才能生效。

> 基于本地 daemon `tools/list` 实时获取，非文档抓取。最后同步：2026-07-13

## 工具一览

### Agents（11 个）

| 工具 | 功能 |
|---|---|
| `create_agent` | 创建 agent，可指定关系（subagent/detached）、工作区、provider/model 及初始 prompt |
| `send_agent_prompt` | 向运行中的 agent 发送任务 |
| `get_agent_status` | 返回 agent 最新状态快照（生命周期、能力、待处理权限） |
| `list_agents` | 列出最近 agent 的紧凑元数据 |
| `cancel_agent` | 中止 agent 的当前 run，但保留 agent 存活 |
| `archive_agent` | 软删除 agent，中断运行中的 agent 并从活动列表移除 |
| `kill_agent` | 永久终止 agent 会话 |
| `update_agent` | 更新 agent 名称、标签或运行时配置（mode / model / thinking / features） |
| `rename_workspace` | 重命名工作区标题（省略 workspaceId 则重命名当前工作区） *← 新增* |
| `get_agent_activity` | 返回 agent 最近时间线的整理摘要 |
| `set_agent_mode` | 切换 agent 的会话模式（plan / bypassPermissions / read-only / auto 等） |

### Browser（22 个）*← 全新分类*

| 工具 | 功能 |
|---|---|
| `browser_list_tabs` | 列出当前工作区的所有浏览器标签页 |
| `browser_new_tab` | 创建新的浏览器标签页（后台打开，不切换用户视图） |
| `browser_close_tab` | 关闭浏览器标签页，移除 webview 并注销 |
| `browser_navigate` | 导航浏览器标签页到指定 URL |
| `browser_snapshot` | 返回浏览器页面的模型可读快照（含元素引用 ref） |
| `browser_click` | 点击浏览器页面中的元素 |
| `browser_fill` | 填充输入框类元素 |
| `browser_type` | 向元素（或焦点元素）输入文本 |
| `browser_keypress` | 向元素（或焦点元素）发送按键 |
| `browser_select` | 设置 select 元素的值 |
| `browser_hover` | 悬停在浏览器元素上 |
| `browser_drag` | 将一个元素拖放到另一个元素上 |
| `browser_scroll` | 按 deltaX/deltaY CSS 像素滚动浏览器标签页 |
| `browser_resize` | 调整浏览器标签页的 webview 视口尺寸 |
| `browser_back` | 浏览器后退 |
| `browser_forward` | 浏览器前进 |
| `browser_reload` | 刷新浏览器标签页 |
| `browser_screenshot` | 截取浏览器标签页的 PNG 截图（支持全页） |
| `browser_wait` | 等待浏览器标签页出现指定文本或到达指定 URL |
| `browser_logs` | 读取浏览器标签页最近的 console 消息和网络性能条目 |
| `browser_evaluate` | 在浏览器标签页中执行 JavaScript 函数 |
| `browser_upload` | 将工作区文件设置到文件 input 元素上 |

### Terminals（5 个）

| 工具 | 功能 |
|---|---|
| `list_terminals` | 列出指定工作目录（或全部工作目录）的终端会话 |
| `create_terminal` | 为工作目录创建终端会话 |
| `kill_terminal` | 终止终端会话 |
| `capture_terminal` | 捕获终端会话的纯文本输出 |
| `send_terminal_keys` | 向终端会话发送文本或特殊按键 |

### Schedules（9 个）

| 工具 | 功能 |
|---|---|
| `create_schedule` | 创建周期性 schedule（按 cron 表达式启动新 agent） |
| `create_heartbeat` | 创建周期性心跳（按 cron 表达式向当前 agent 发送 prompt） *← 新增* |
| `list_schedules` | 列出 daemon 管理的所有 schedule |
| `inspect_schedule` | 查看 schedule 详情及运行历史 |
| `pause_schedule` | 暂停运行中的 schedule |
| `resume_schedule` | 恢复已暂停的 schedule |
| `update_schedule` | 更新 schedule 配置（cron / provider / model / prompt 等） *← 新增* |
| `delete_schedule` | 永久删除 schedule |
| `schedule_logs` | 获取 schedule 的运行历史日志 *← 新增* |

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
| `list_pending_permissions` | 返回所有 agent 的待处理权限请求（含标准化载荷） |
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

总计 **56 个 MCP 工具**，分布于 **8 个分类**。

## 与旧版文档差异

| 变更 | 说明 |
|---|---|
| ➕ **Browser 分类**（22 个工具） | 旧文档完全未收录。Paseo 内置 Playwright 浏览器自动化能力 |
| ➕ `rename_workspace` | Agents 分类新增 |
| ➕ `create_heartbeat` | Schedules 分类新增（向当前 agent 发送 cron 心跳，而非启动新 agent） |
| ➕ `update_schedule` | Schedules 分类新增（在线修改 schedule 配置） |
| ➕ `schedule_logs` | Schedules 分类新增（查看 schedule 运行历史） |
| ❌ `wait_for_agent` | 旧文档误收录，daemon 实际无此工具 |
