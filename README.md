[English](./README_EN.md) | **中文**

# paseo-notifier

Paseo Agent 状态通知器。

当前 v0.1.102 的电脑版本(windows)和手机版本(Android)都在声音通知上存在异常，消息完成后无法进行有效提示.

经过多种方法并没有解决，但是无法通知就代表没法了解任务的运行状态，这是很影响效率的.

通过 MCP API 轮询 Paseo 守护进程中的 Agent 状态，在任务完成、出错、需要交互、疑似卡死、卡死确认、活动恢复以及运行中心跳等场景下，通过配置的渠道发送通知。

同时通过 **WebSocket 实时接收** Paseo daemon 的 Provider Subagent 推送，追踪子任务的启动、运行中、全部完成等状态变化，在子任务完成时自动触发主 Agent 继续执行。

支持多供应商并行通知，内置控制台输出、钉钉机器人、飞书 Webhook、飞书自应用。基于 notify 库的插件机制可轻松扩展 Slack、Telegram、Discord 等 30+ 通知渠道（详见下文）。

## 快速安装

```bash
go install github.com/winezer0/paseo-notifier/cmd/paseo-notifier@latest
```

安装后二进制在 `$GOPATH/bin`（或 `$GOBIN`），继续配置和启动：

```bash
paseo-notifier --init          # 生成配置文件到程序所在目录
# 编辑 paseo-notifier.yaml 填入通知配置
paseo-notifier install         # 注册为系统服务
paseo-notifier start           # 启动服务
```


## 架构

### 数据流

```
Paseo Daemon
  │
  ├── MCP API (127.0.0.1:6767/mcp/agents)
  │     │  每 5s 轮询
  │     ▼
  │   Agent Watcher
  │     ├── list_agents              → 检测 finished / error / stuck
  │     ├── get_agent_activity       → 活动摘要附加到通知
  │     ├── checkRunningAgents       → 运行中状态心跳通知
  │     └── checkRunningSubagents    → subagent 持续运行通知
  │
  ├── WebSocket (ws://127.0.0.1:6767/ws)
  │     │  实时推送
  │     ▼
  │   ProviderSubagentTracker
  │     ├── provider_subagents.update  → 子任务启动/完成检测
  │     ├── subagent_spawned           → 首次出现通知
  │     ├── all_subagents_done         → 全部完成通知
  │     └── auto_continue              → 完成后自动继续主 Agent
  │
  └── Notifier.Notify(event)
        ├── 控制台输出
        ├── 钉钉机器人
        ├── 飞书 Webhook
        └── 飞书自应用
```

### 事件类型

| 事件 | 触发条件 | 检测方式 |
|:---|:---|:---|
| ✅ 任务完成 | `attentionReason: null → "finished"`（无运行中子任务时） | `list_agents` |
| ❌ 运行出错 | `attentionReason: null → "error"` | `list_agents` |
| ⚠️ 需要交互 | `list_pending_permissions` 出现新项 | 权限请求列表 |
| 🔔 疑似卡死警告 | `UpdatedAt` 超过 `stuck_detect_timeout` 无变化 | `get_agent_activity` 确认前发出警告 |
| 🔔 Agent 卡死 | `get_agent_activity` 确认最后活动也已超时 | 二次确认后发送 |
| ℹ️ 活动正常 | 二次确认发现 Agent 仍在活动 | `get_agent_activity` 确认后发送 |
| 🔄 运行中状态 | Agent 持续运行超过 `running_status_interval` | `checkRunningAgents`（附子任务汇总） |
| 🚀 子任务启动 | 首次检测到 subagent 出现 | WebSocket 推送 |
| 🎉 子任务全部完成 | 所有 subagent 状态变为非 running | WebSocket 推送 |
| 🔄 子任务持续运行 | subagent 持续运行超过间隔 | 轮询检查 |
| 🔔 自动继续 | 任务完成含继续关键词 / 子任务完成后主 Agent 空闲 | `auto_continue` |
| 🔔 卡死恢复尝试 | 卡死后自动发送恢复提示 | `stuck_restart` |
| 🔔 启动通知 | 通知器启动时 | 首次初始化 |
| 🔌 断连/重连 | MCP 守护进程连接断开/恢复 | 轮询请求失败/成功 |

### 活动摘要

**finished / error** 事件触发时，自动获取 Agent 最近的活动记录，将最后 1 条摘要（截断至 80 字符）附加到通知中。

### Provider Subagent 追踪

paseo-notifier 通过 WebSocket（`ws://127.0.0.1:6767/ws`）连接 Paseo daemon，声明 `provider_subagents` capability 后实时接收子任务状态推送。

**覆盖范围**：Paseo 管理子 agent（`create_agent(relationship: subagent)`）、Claude/Codex/OpenCode 原生子会话。

**三种通知**：

| 通知 | 触发 | 频率控制 |
|---|---|---|
| 🚀 子任务启动 | 父 agent 首次出现 subagent | 每 parent 仅首次 |
| 🔄 持续运行中 | subagent 一直在跑 | 默认每 3 分钟 |
| 🎉 全部完成 | 所有 subagent 非 running | 每轮子任务仅一次 |

**虚假完成抑制**：父 agent finished 但还有子任务在运行时，不发送"任务完成"通知。

**自动继续**：当 `auto_continue: true` 且子任务全部完成时，若父 agent 处于 idle/finished 状态，自动发送继续提示。

### 自动继续（Auto Continue）

两种触发场景：

1. **任务完成时**：Agent finished 后，若最后一条活动包含"继续"/"continue"等关键词，自动发送继续提示
2. **子任务完成时**：子任务全部完成后，若主 Agent 处于空闲状态，自动发送继续提示唤醒

仅在含关键词或子任务完成时触发，不会盲目自动继续。

当正在运行的 Agent 的 `UpdatedAt` 字段超过 `stuck_detect_timeout`（默认 120 秒）没有变化时：

1. **疑似卡死警告**：第一时间发送警告通知 "Agent 可能卡死（正在确认）"，告知用户已进入检查流程。
2. **二次确认**：调用 `get_agent_activity` 获取 Agent 最新的活动记录，判断最后活动时间是否也已超时。
3. **判定结果**：
   - 最后活动也已超时 → 发送**确认卡死**通知，附空闲时长、卡死原因和活动记录摘要
   - 最后活动仍在超时阈值内 → 发送**活动正常**通知，附最近活动记录，说明 Agent 仍在工作中

检测到卡死后，可配置自动重启流程（`stuck_restart_delay` / `stuck_restart_retry`）：
1. 达到 `stuck_detect_timeout` 后发送卡死警告
2. `stuck_restart_delay` 后再次检查 Agent 是否自行恢复
3. 仍未恢复则自动发送 `continue` 提示尝试恢复 Agent
4. 重试达 `stuck_restart_retry` 次后放弃

> 如果 `UpdatedAt` 在监控期间恢复正常更新，卡死状态会自动重置。

### 运行中状态心跳

当 Agent 持续运行超过 `running_status_interval`（默认 5 分钟）且用户无新交互时，会定期发送运行中状态心跳通知。通知内容包括 Agent 基本信息、运行时长和最近活动记录，方便你在 Agent 长时间运行时仍能掌握其工作进度。

### 自动继续（Auto Continue）

当启用 `auto_continue: true` 时，Agent 任务完成后会检查其最后一条活动记录是否包含 `继续` / `continue` 等关键词。如果命中，自动调用 `send_agent_prompt` 发送继续提示，适用于大模型执行完一轮后询问"是否继续"的场景。

仅在 **含关键词** 时触发，不会盲目自动继续，误触发率低。

### 通知类型

| 通知 | 触发时机 | 发送渠道 |
|:---|:---|:---|
| 任务完成 | Agent 任务正常结束（无运行中子任务） | 所有已配置供应商 |
| 任务出错 | Agent 执行出错 | 所有已配置供应商 |
| 权限请求 | Agent 需要用户确认 | 所有已配置供应商 |
| 疑似卡死警告 | UpdatedAt 超过 `stuck_detect_timeout` 无变化 | 所有已配置供应商 |
| 确认卡死 | 二次确认后仍判定卡死 | 所有已配置供应商 |
| 活动正常 | 二次确认发现 Agent 仍在运行 | 所有已配置供应商 |
| 运行中状态 | Agent 持续运行超过 `running_status_interval` | 所有已配置供应商 |
| 子任务启动 | 首次检测到 subagent | 所有已配置供应商 |
| 子任务全部完成 | 所有 subagent 完成 | 所有已配置供应商 |
| 子任务持续运行 | subagent 持续运行超过间隔 | 所有已配置供应商 |
| 自动继续 | 任务完成 / 子任务完成后触发继续 | 所有已配置供应商 |
| 卡死恢复尝试 | 卡死后自动发送恢复提示 | 所有已配置供应商 |
| 启动通知 | 通知器启动时 | 所有已配置供应商 |
| 断连通知 | MCP 守护进程连接断开 | 所有已配置供应商 |
| 重连通知 | MCP 守护进程连接恢复 | 所有已配置供应商 |

### 重复通知防护

- **finished / error**：对比 `(attentionReason, attentionTimestamp)`，相同则不重复
- **权限请求**：记录已通知的 permission ID
- **卡死相关**：卡死警告、活动正常、确认卡死均仅首次发送；`UpdatedAt` 恢复更新后自动重置状态
- **运行中状态**：每轮 `running_status_interval` 间隔最多发送一次通知，不重复
- **断连重连**：重连后清除历史状态快照，避免断连期间堆积的重复通知
- **归档 Agent**：`archivedAt` 已设置的 Agent 跳过
- **Subagent**：`spawnNotified`/`allDoneNotified` 标记去重；`Reset()` 保留通知状态避免重连重复
- **自动继续**：任务完成时仅含关键词触发；子任务完成时仅父 agent 空闲触发
- **子任务持续运行**：每 `subagent_running_interval` 间隔最多一次通知
- **活动摘要**：超 80 字符自动截断，多行只取首行

## 配置

### 配置文件搜索顺序

1. `--config` 参数指定的路径
2. **程序所在目录**的 `paseo-notifier.yaml`
3. 内置默认配置（仅日志输出）

> **注意**：配置文件和日志文件都只在程序所在目录查找，不依赖用户主目录。
> 这样无论前台运行还是 SYSTEM 账户服务运行，都使用同一份配置和日志，避免不同用户权限导致的问题。

### 生成配置文件

`--init` 将完整的默认配置文件（带注释）写入程序所在目录：

```bash
# 写入 <程序目录>/paseo-notifier.yaml
paseo-notifier --init

# 写入指定路径
paseo-notifier --config /path/to/custom.yaml --init
```

### 完整配置示例

```yaml
# 监控相关配置
monitor:
  daemon_url: "http://127.0.0.1:6767"
  # WebSocket 地址覆盖（空则自动从 daemon_url 推导 ws://host:port/ws）
  # ws_url: ""
  # 轮询间隔（Go time.Duration 格式）
  interval: "5s"
  # 卡死检测超时，0s/false/空 = 禁用，默认 120s
  stuck_detect_timeout: 120s
  # 检测到卡死后延迟重启，0s/false/空 = 禁用，默认 0s
  stuck_restart_delay: 0s
  # 自动重启最大重试次数，默认 5
  stuck_restart_retry: 5
  # 运行中状态心跳通知间隔（Go time.Duration 格式），默认 5m
  running_status_interval: 5m
  # 子任务持续运行通知间隔（Go time.Duration 格式），默认 3m
  subagent_running_interval: 3m
  # 任务完成后自动继续（true/false，默认 false）
  # 当 Agent 最后一条活动包含"继续"等关键词时，自动发送继续提示
  auto_continue: false

# 通用配置（日志、语言等）
common:
  # 日志级别，可选 debug/info/warn/error（默认 info）
  log_level: "info"
  # 日志文件路径（默认：程序所在目录下的 paseo-notifier.log）
  # 留空则使用默认路径，日志文件超 10MB 自动轮转清理
  log_file: ""
  # 控制台日志格式，T=时间 L=级别 C=调用者 M=消息（默认 TLM）
  # 设为 false 或 off 关闭控制台输出
  log_console: "TLM"
  # 通知消息语言
  # auto: 自动检测系统语言（中文系统→zh，其他→en）
  # zh:   中文
  # en:   英文
  language: "auto"

# 通知供应商配置
# providers 是一个数组，可以同时配置多个供应商
# 不配置任何供应商时，事件仅输出到日志
notifier:
  providers:
    # 控制台输出（带 emoji 格式），enable: true 时启用
    - type: "console"
      config:
        enable: true

    # 钉钉自定义机器人（Webhook + 加签）
    # - type: "dingtalk"
    #   config:
    #     access_token: ""
    #     secret: ""

    # 飞书群聊机器人（Webhook 模式）
    # - type: "lark_webhook"
    #   config:
    #     webhook_url: "https://open.feishu.cn/open-apis/bot/v2/hook/your-webhook-id"

    # 飞书自应用（支持指定接收者）
    # - type: "lark_app"
    #   config:
    #     app_id: "cli_your-app-id"
    #     app_secret: "your-app-secret"
    #     receivers:
    #       - type: chat_id
    #         value: "oc_your-chat-id"
```

`providers` 是一个数组，支持同时配置多个供应商。所有启用的供应商会同时发送通知。

### 供应商类型

| 类型 | 说明 | 配置参数 |
|:---|:---|:---|
| `console` | 控制台输出（带 emoji 格式） | `enable`（bool） |
| `dingtalk` | 钉钉自定义机器人（Webhook + 加签） | `access_token`, `secret` |
| `lark_webhook` | 飞书群聊机器人（Webhook） | `webhook_url` |
| `lark_app` | 飞书自应用（支持指定接收者） | `app_id`, `app_secret`, `receivers[]`（type: open_id/user_id/union_id/email/chat_id, value） |

### 默认行为

不配置任何通知供应商时，Agent 事件仅输出到日志（控制台和文件），不发送外部通知。


## 使用

### 编译

```bash
# 完整版（含系统服务管理功能）
go build -o paseo-notifier.exe ./cmd/paseo-notifier

# 简化版（无服务管理依赖，仅前台运行）
go build -tags noservice -o paseo-notifier.exe ./cmd/paseo-notifier
```

| 构建方式 | 服务管理命令 | 适用场景 |
|----------|-------------|----------|
| 默认 | install / uninstall / start / stop / restart | 需要系统服务管理、开机自启 |
| `-tags noservice` | 不可用 | 仅前台运行，二进制更小 |

### 生成配置文件

```bash
# 写入 <程序目录>/paseo-notifier.yaml，编辑后直接使用
paseo-notifier --init
```

### 运行方式

```bash
# 前台运行
paseo-notifier

# 查看版本
paseo-notifier --version

# 指定配置文件
paseo-notifier --config /path/to/config.yaml

# 覆盖日志级别（debug/info/warn/error）
paseo-notifier --ll debug

# 自定义控制台日志格式（T=时间 L=级别 C=调用者 F=函数 M=消息）
paseo-notifier --lc "TLCM"

# 指定日志文件路径
paseo-notifier --lf /path/to/custom.log
```

### 查看日志

日志文件默认写入程序所在目录：

```bash
# Windows
type <程序目录>\paseo-notifier.log

# Linux/macOS
tail -f <程序目录>/paseo-notifier.log
```

### 系统服务（完整版）

利用 [kardianos/service](https://github.com/kardianos/service) 实现跨平台服务管理。

#### Windows

以管理员身份打开 PowerShell：

```powershell
# 1. 生成配置文件（写入程序所在目录）
.\paseo-notifier.exe --init

# 2. 编辑 .\paseo-notifier.yaml 填入通知配置

# 3. 注册为 Windows 服务并启动
.\paseo-notifier.exe install
.\paseo-notifier.exe start

# 4. 查看服务状态
Get-Service paseo-notifier

# 5. 停止 / 重启
.\paseo-notifier.exe stop
.\paseo-notifier.exe restart

# 6. 卸载服务
.\paseo-notifier.exe stop
.\paseo-notifier.exe uninstall
```

安装后，Windows 服务管理器 (`services.msc`) 中会出现名为 **paseo-notifier** 的服务，可设置开机自启类型为"自动"。

#### Linux (systemd)

```bash
# 1. 生成配置文件（写入程序所在目录）
sudo ./paseo-notifier --init

# 2. 编辑 ./paseo-notifier.yaml 填入通知配置

# 3. 注册 systemd 服务
sudo ./paseo-notifier install

# 4. 启动 / 设置开机自启
sudo ./paseo-notifier start
sudo systemctl enable paseo-notifier

# 5. 查看状态
sudo systemctl status paseo-notifier

# 6. 停止 / 重启
sudo ./paseo-notifier stop
sudo ./paseo-notifier restart

# 7. 卸载
sudo ./paseo-notifier stop
sudo ./paseo-notifier uninstall
```

#### macOS (Launchd)

```bash
# 注册 Launchd 服务
sudo ./paseo-notifier install

# 启动 / 停止 / 卸载
sudo ./paseo-notifier start
sudo ./paseo-notifier stop
sudo ./paseo-notifier uninstall
```

## 开发

### 添加自定义通知供应商

`message/` 包实现了供应商注册机制。添加新供应商只需两步：

1. 在 `message/` 包下创建新文件（如 `provider_slack.go`）
2. 通过 `init()` 函数调用 `RegisterProvider` 注册工厂

```go
// message/provider_slack.go
package message

import (
    "errors"

    "github.com/nikoksr/notify"
    "github.com/nikoksr/notify/service/slack"
    "gopkg.in/yaml.v3"
)

func init() {
    RegisterProvider("slack", newSlackProvider)
}

type slackConfig struct {
    Token   string `yaml:"token"`
    Channel string `yaml:"channel"`
}

func newSlackProvider(rawCfg yaml.Node) (notify.Notifier, error) {
    var cfg slackConfig
    if err := rawCfg.Decode(&cfg); err != nil {
        return nil, err
    }
    if cfg.Token == "" {
        return nil, errors.New("slack: token is required")
    }
    svc := slack.New(cfg.Token)
    svc.AddReceivers(cfg.Channel)
    return svc, nil
}
```

无需修改核心代码，`init()` 自动完成注册，`notifier.go` 通过注册中心查找并调用工厂。

底层 Notify 库支持 30+ 种通知服务，包括 Slack、Telegram、Discord、邮件等。完整列表见 [nikoksr/notify](https://github.com/nikoksr/notify)。

## 依赖

- [nikoksr/notify](https://github.com/nikoksr/notify) — 多通道通知库
- [kardianos/service](https://github.com/kardianos/service) — 跨平台系统服务管理
- [go-flags](https://github.com/jessevdk/go-flags) — 命令行参数解析
- [yaml.v3](https://gopkg.in/yaml.v3) — YAML 配置解析
