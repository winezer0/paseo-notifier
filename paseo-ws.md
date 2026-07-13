# Paseo WebSocket 协议

Paseo daemon 通过 WebSocket 向客户端（桌面应用、移动应用、CLI、第三方工具）提供实时通信能力。paseo-notifier 通过此协议接收 provider subagent 状态推送。

> 基于 Paseo 源码 `packages/server/src/server/websocket-server.ts` + `packages/protocol/src/messages.ts` + **实测验证**。最后同步：2026-07-13

## 连接

| 项目 | 值 |
|---|---|
| **端点** | `ws://<host>:<port>/ws`（默认 `ws://127.0.0.1:6767/ws`） |
| **协议版本** | `WS_PROTOCOL_VERSION = 1` |
| **Hello 超时** | 15 秒（超时关闭码 `4001`） |
| **无效 Hello 关闭码** | `4002` |
| **认证失败关闭码** | `4401` |
| **不兼容协议关闭码** | `4003` |
| **服务关闭码** | `1001` |
| **消息格式** | JSON 文本帧，结构 `{ type, requestId?, payload? }` |
| **二进制帧** | 支持（用于文件上传等场景） |

### 连接流程

```
Client                           Daemon
  │                                │
  │──── TCP upgrade /ws ──────────►│
  │                                │  verifyClient (origin/host 校验)
  │                                │  handleProtocols (auth bearer token)
  │◄─── 101 Switching Protocols ───│
  │                                │
  │──── Hello 消息 ───────────────►│  15s 超时
  │                                │  验证 protocol version
  │◄─── HelloResponse ────────────│  返回 server_info + capabilities
  │                                │
  │──── 业务消息 ─────────────────►│
  │◄─── 推送 / 响应 ──────────────│
```

## Hello 握手

### Client → Server（Hello）

> ⚠️ 以下字段约束来自 daemon 源码 `WSHelloMessageSchema`（Zod），**全部必填字段缺一不可**，否则 daemon 立即 `close 4002: Invalid hello`。

```json
{
  "type": "hello",
  "protocolVersion": 1,
  "clientId": "paseo-notifier",
  "clientType": "cli"
}
```

可选字段：

```json
{
  "appVersion": "0.0.9",
  "capabilities": {
    "provider_subagents": true
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | `"hello"` | ✅ | 固定值 |
| `protocolVersion` | number | ✅ | 必须 = `1`（`WS_PROTOCOL_VERSION`） |
| `clientId` | string | ✅ | 非空字符串，客户端实例标识 |
| `clientType` | `"mobile" \| "browser" \| "cli" \| "mcp"` | ✅ | 客户端类型枚举 |
| `appVersion` | string | ❌ | 客户端版本号 |
| `capabilities` | `Record<string, unknown>` | ❌ | 能力声明，key 使用下划线格式（如 `provider_subagents`） |

### 实测踩坑

| 错误 | 原因 | daemon 返回 |
|---|---|---|
| 缺 `protocolVersion` | Hello Schema 校验失败 | `close 4002` |
| 缺 `clientId` | `message.clientId.trim().length === 0` | `close 4002` |
| 缺 `clientType` | Zod `z.enum(...)` 不带 `.optional()`，缺少直接校验失败 | `close 4002` |
| capability key 用驼峰 `providerSubagents` | daemon 检查 `capabilities["provider_subagents"]`（下划线），驼峰匹配不上 | 无报错，但不推送 subagent 消息 |

### Server → Client（Hello Response）

```json
{
  "type": "hello_response",
  "serverId": "server-uuid",
  "protocolVersion": 1,
  "daemonVersion": "0.1.107",
  "capabilities": {
    "voice": { "dictation": { "enabled": false }, "voice": { "enabled": false } }
  },
  "features": {
    "daemonSelfUpdate": true,
    "providerSubagents": true,
    "daemonDiagnostics": true
  }
}
```

## Client Capabilities（客户端能力声明）

| 常量名 | Wire Key | 类型 | 说明 |
|---|---|---|---|
| `CLIENT_CAPS.providerSubagents` | `"provider_subagents"` | boolean | 声明后服务端推送 `agent.provider_subagents.update` |
| `CLIENT_CAPS.browserHost` | `"browser_host"` | `{ available: boolean }` | 声明浏览器自动化宿主能力 |

> ⚠️ Wire key 使用**下划线**格式（`provider_subagents`），不是驼峰（`providerSubagents`）。daemon 通过 `CLIENT_CAPS` 常量映射校验。

## Server Features（服务端特性标志）

| Feature Key | 说明 |
|---|---|
| `providerSubagents` | 支持 provider subagent 协议（PR #2013） |
| `daemonSelfUpdate` | 支持远程 daemon 自更新 |
| `daemonDiagnostics` | 支持诊断报告 |
| `browserTools` | 支持浏览器自动化工具 |

## 消息格式

### Session 信封（⚠️ 关键）

daemon 通过 `wrapSessionMessage()` 将**所有** Session 层出站消息包装在 `session` 信封中：

```typescript
// packages/protocol/src/messages.ts line 4732
export function wrapSessionMessage(sessionMsg: SessionOutboundMessage): WSOutboundMessage {
  return {
    type: "session",
    message: sessionMsg,   // ← 真实消息嵌套在此字段中
  };
}
```

**服务端发出的每条消息都是两层结构**：

```json
{
  "type": "session",
  "message": {
    "type": "agent.provider_subagents.update",
    "payload": { "parentAgentId": "...", "status": "running" }
  }
}
```

客户端接收时必须**先解包 `session` 信封**，再从 `message` 字段中取出实际消息类型和载荷进行分发。

> 这是实测中导致"WS 连上了但收不到任何 subagent 通知"的根因——直接监听 `agent.provider_subagents.update` 会漏掉所有消息，因为 wire type 永远是 `"session"`。

### 入站消息（Client → Server）

### 信封结构

所有消息共用此 JSON 信封：

```json
{
  "type": "message.type.identifier",
  "requestId": "optional-correlation-id",
  "payload": { }
}
```

- `type` — 消息类型，使用点号分隔的命名空间（如 `agent.provider_subagents.update`）
- `requestId` — 请求-响应关联 ID，仅 RPC 消息携带
- `payload` — 消息载荷，类型特定

### RPC 模式（请求-响应）

```
Client → { type: "xxx.request", requestId: "r1", payload: {...} }
Server → { type: "xxx.response", requestId: "r1", payload: {...} }
```

- 客户端生成的 `requestId` 会被原样回传
- 请求和响应的 `type` 对应（`.request` ↔ `.response`）

### Push 模式（服务端推送）

```
Server → { type: "agent.provider_subagents.update", payload: {...} }
```

无 `requestId`，服务端主动推送。

---

## 消息类型一览

### Agent 管理

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `create_agent_request` / `response` | C→S / S→C | 创建 agent（支持 subagent/detached + worktree） |
| `fetch_agents_request` / `response` | C→S / S→C | 获取 agent 列表（分页、筛选、排序、订阅） |
| `fetch_agent_request` / `response` | C→S / S→C | 获取单个 agent 详情 |
| `send_agent_message` | C→S | 向 agent 发送消息（fire-and-forget） |
| `send_agent_message_request` / `response` | C→S / S→C | 向 agent 发送消息（RPC，等待确认） |
| `wait_for_finish_request` / `response` | C→S / S→C | 阻塞等待 agent 完成或请求权限 |
| `abort_request` | C→S | 中止当前请求 |
| `delete_agent_request` / `response` | C→S / S→C | 删除 agent |
| `archive_agent_request` / `response` | C→S / S→C | 归档 agent（软删除） |
| `update_agent_request` / `response` | C→S / S→C | 更新 agent 名称/标签 |
| `close_items_request` / `response` | C→S / S→C | 批量关闭 agent 和终端 |

### Agent 事件推送（Push）

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `session.message` | S→C | 包装后的会话消息（所有 push 的统一信封） |
| `agent.attention_required` | S→C | Agent 需要关注（finished/error/permission） |
| `agent.stream_event` | S→C | Agent 流事件（turn 生命周期、timeline、权限） |

`agent.stream_event` 内部 `type`：

| 子类型 | 说明 |
|---|---|
| `thread_started` | Agent 线程启动 |
| `turn_started` | 一轮执行开始 |
| `turn_completed` | 一轮执行完成 |
| `turn_failed` | 一轮执行失败 |
| `turn_canceled` | 一轮执行被取消 |
| `timeline` | 时间线条目（消息、推理、工具调用等） |
| `permission_requested` | 权限请求 |
| `permission_resolved` | 权限被处理 |
| `attention_required` | 需要用户关注 |

### Provider Subagent（PR #2013）

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `agent.provider_subagents.list.request` / `response` | C→S / S→C | 获取 provider subagent 描述符列表 |
| `agent.provider_subagents.timeline.get.request` / `response` | C→S / S→C | 获取 subagent 时间线 |
| `agent.provider_subagents.update` | S→C | Provider subagent 实时状态更新（Push） |

详见 [Provider Subagent 协议](#provider-subagent-协议) 章节。

### 工作区（Workspace）

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `fetch_workspaces_request` / `response` | C→S / S→C | 获取工作区列表 |
| `workspace.title.set.request` / `response` | C→S / S→C | 设置工作区标题 |

### 项目（Project）

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `project.rename.request` / `response` | C→S / S→C | 重命名项目 |
| `project.remove.request` / `response` | C→S / S→C | 移除项目 |

### Provider 管理

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `list_available_providers_request` / `response` | C→S / S→C | 列出可用 provider |
| `list_provider_models_request` / `response` | C→S / S→C | 列出 provider 模型 |
| `list_provider_modes_request` / `response` | C→S / S→C | 列出 provider 模式 |
| `get_providers_snapshot_request` / `response` | C→S / S→C | 获取 provider 快照 |
| `refresh_providers_snapshot_request` / `response` | C→S / S→C | 刷新 provider 快照 |

### Session 导入

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `fetch_recent_provider_sessions_request` / `response` | C→S / S→C | 获取近期 provider 会话 |
| `import_session.request` / `response` | C→S / S→C | 导入已有 provider 会话 |

### Daemon 管理

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `daemon.get_status.request` / `response` | C→S / S→C | 获取 daemon 状态 |
| `daemon.get_pairing_offer.request` / `response` | C→S / S→C | 获取配对信息 |
| `daemon.update.request` / `response` | C→S / S→C | 远程更新 daemon |
| `daemon.update.progress` | S→C | 更新进度推送（Push） |
| `diagnostics.request` / `response` | C→S / S→C | 获取诊断报告 |

### 配置管理

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `get_daemon_config_request` / `response` | C→S / S→C | 读取 daemon 配置 |
| `set_daemon_config_request` / `response` | C→S / S→C | 修改 daemon 配置 |
| `read_project_config_request` / `response` | C→S / S→C | 读取项目配置 |
| `write_project_config_request` / `response` | C→S / S→C | 写入项目配置 |

### 语音（Voice）

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `set_voice_mode` / `response` | C→S / S→C | 切换语音模式 |
| `voice_audio_chunk` | C→S | 音频数据块（base64） |
| `audio_played` | C→S | 音频播放完成确认 |
| `dictation_stream_start` / `finish` / `cancel` | C→S | 听写流控制 |
| `dictation_stream_chunk` | C→S | 听写音频块 |

### Chat

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `chat.create.request` / `response` | C→S / S→C | 创建聊天 |
| `chat.list.request` / `response` | C→S / S→C | 聊天列表 |
| `chat.read.request` / `response` | C→S / S→C | 读取聊天 |
| `chat.post.request` / `response` | C→S / S→C | 发送聊天消息 |
| `chat.wait.request` / `response` | C→S / S→C | 等待聊天回复 |
| `chat.delete.request` / `response` | C→S / S→C | 删除聊天 |

### 浏览器自动化

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `browser.automation.execute_request` / `response` | C→S / S→C | 远程执行浏览器自动化命令 |

### 终端关注

| 消息类型 | 方向 | 说明 |
|---|---|---|
| `terminal_attention_required` | S→C | 终端需要关注（完成/需要输入） |

---

## Provider Subagent 协议

PR #2013 引入，用于将 provider 原生子会话（Claude/Codex/OpenCode 的子 agent）投影到 Paseo 交互窗口。paseo-notifier 利用此协议实现"全部子任务完成"通知。

### 能力门控

客户端必须在 Hello 中声明 `"providerSubagents": true` 才能接收相关推送。

### 消息详述

#### `agent.provider_subagents.list.request` → `response`

获取指定父 agent 的所有 provider subagent 描述符。

**请求**：
```json
{
  "type": "agent.provider_subagents.list.request",
  "requestId": "req-1",
  "payload": { "parentAgentId": "agent-abc123" }
}
```

**响应**：
```json
{
  "type": "agent.provider_subagents.list.response",
  "requestId": "req-1",
  "payload": {
    "subagents": [
      {
        "parentAgentId": "agent-abc123",
        "subagentId": "sub-xyz789",
        "title": "build auth module",
        "provider": "codex",
        "model": "gpt-5.4",
        "status": "running"
      }
    ]
  }
}
```

#### `agent.provider_subagents.update`（Push）

当 subagent 状态变化时服务端主动推送。

```json
{
  "type": "agent.provider_subagents.update",
  "payload": {
    "parentAgentId": "agent-abc123",
    "subagentId": "sub-xyz789",
    "title": "build auth module",
    "provider": "codex",
    "model": "gpt-5.4",
    "status": "completed"
  }
}
```

**status 取值**：`running` | `idle` | `completed` | `error`

#### `agent.provider_subagents.timeline.get.request` → `response`

获取 subagent 的完整时间线（消息、推理、工具调用等）。

### 在 paseo-notifier 中的使用

```
Daemon WS Push: agent.provider_subagents.update
    │
    ▼
DaemonWSClient (agentwatcher/wsclient.go)
    │ OnMessage("agent.provider_subagents.update", handler)
    ▼
ProviderSubagentTracker (agentwatcher/provider_tracker.go)
    │ 更新状态；检测 parent 下是否全部 completed/idle/error
    │ 全部完成 + 未通知 → 触发回调
    ▼
Watcher → Notifier → 通知渠道
```

---

## Timeline Item 类型

Agent 时间线中的条目类型（`timeline` 事件的 `item` 字段）：

| type | 说明 |
|---|---|
| `user_message` | 用户消息 |
| `assistant_message` | 助手消息 |
| `reasoning` | 推理过程 |
| `tool_call` | 工具调用（含 detail 子类型） |
| `todo` | 待办事项列表 |
| `error` | 错误 |
| `compaction` | 上下文压缩 |

`tool_call` 的 `detail.type`：

| 子类型 | 说明 |
|---|---|
| `shell` | Shell 命令执行 |
| `read` | 文件读取 |
| `edit` | 文件编辑 |
| `write` | 文件写入 |
| `search` | 搜索操作 |
| `fetch` | HTTP 请求 |
| `sub_agent` | 子 agent 调用 |
| `plan` | 执行计划 |
| `worktree_setup` | Worktree 初始化 |
| `plain_text` | 纯文本工具 |
| `unknown` | 未知工具 |

---

## Agent 状态枚举

`AgentSnapshotPayload.status` 取值（`AGENT_LIFECYCLE_STATUSES`）：

| 值 | 说明 |
|---|---|
| `initializing` | 初始化中 |
| `idle` | 空闲（等待用户输入） |
| `running` | 运行中 |
| `error` | 出错 |
| `closed` | 已关闭 |

`attentionReason` 取值：

| 值 | 说明 |
|---|---|
| `finished` | 任务完成 |
| `error` | 任务出错 |
| `permission` | 需要权限确认 |

---

## 与 MCP API 的关系

| | WebSocket | MCP（`/mcp/agents`） |
|---|---|---|
| 数据方向 | 双向实时推送 | 请求-响应轮询 |
| Subagent 数据 | ✅ `agent.provider_subagents.*` | ❌ 不支持 |
| Agent CRUD | ✅ `create_agent_request` 等 | ✅ `list_agents`、`create_agent` 等 |
| 适用场景 | 实时状态监控 | 一次性查询/操作 |
| paseo-notifier 使用 | 接收 subagent 推送 | 轮询获取 agent 状态、权限 |

两者互补：MCP 做定时轮询获取 agent 元数据，WebSocket 做实时事件接收。
