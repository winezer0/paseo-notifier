[English](./README_EN.md) | **中文**

# paseo-notifier

Paseo Agent 状态通知器。

当前 v0.1.102 的电脑版本(windows)和手机版本(Android)都在声音通知上存在异常，消息完成后无法进行有效提示.

经过多种方法并没有解决，但是无法通知就代表没法了解任务的运行状态，这是很影响效率的.

分析发现通过 MCP API 轮询 Paseo 守护进程中的 Agent 状态，在任务完成、出错或需要用户交互时通过配置的渠道发送通知。

使用notify库，支持多种通知方式，当前支持 dingtalk feishu lark 格式的通知配置，注册其他类型的通知请看下文.

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
守护进程 MCP API (127.0.0.1:6767)
       │
       ▼
  Agent Watcher (每 5s 轮询)
       │
       ├── list_agents                    → 检测 finished / error
       └── list_pending_permissions       → 检测新权限请求
       │
       ▼
  Notifier.Notify(event)
       │
       ▼
  notify.UseServices(svc...)
       │
       ├── 钉钉机器人
       ├── 飞书 Webhook
       └── 飞书自应用
```

### 事件类型

| 事件 | 触发条件 | 检测方式 |
|:---|:---|:---|
| ✅ 任务完成 | `attentionReason: null → "finished"` | `list_agents` |
| ❌ 运行出错 | `attentionReason: null → "error"` | `list_agents` |
| ⚠️ 需要交互 | `list_pending_permissions` 出现新项 | 权限请求列表 |

### 重复通知防护

- **finished / error**：对比 `(attentionReason, attentionTimestamp)`，相同则不重复
- **权限请求**：记录已通知的 permission ID
- **断连重连**：重连后清除历史状态快照，避免断连期间堆积的重复通知
- **归档 Agent**：`archivedAt` 已设置的 Agent 跳过

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
monitor:
  daemon_url: "http://127.0.0.1:6767/mcp/agents"
  interval: "5s"

log_format: "text"

# 日志文件路径（默认：程序所在目录下的 paseo-notifier.log）
# 留空则使用默认路径，日志文件超 10MB 自动轮转清理
log_path: ""

# 是否同时输出日志到控制台（默认 true）
log_console: true

# 通知消息语言（默认 auto，自动检测系统语言）
# 可选：auto / zh / en
language: "auto"

notifier:
  providers:
    - type: "dingtalk"
      config:
        access_token: "your-dingtalk-access-token"
        secret: "your-dingtalk-signing-secret"

    - type: "lark_webhook"
      config:
        webhook_url: "https://open.feishu.cn/open-apis/bot/v2/hook/your-webhook-id"

    - type: "lark_app"
      config:
        app_id: "cli_your-app-id"
        app_secret: "your-app-secret"
        receivers:
          - type: chat_id
            value: "oc_your-chat-id"
```

`providers` 是一个数组，支持同时配置多个供应商。所有启用的供应商会同时发送通知。

### 供应商类型

| 类型 | 说明 | 配置参数 |
|:---|:---|:---|
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
