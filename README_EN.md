**English** | [中文](./README.md)

# paseo-notifier

Paseo Agent status notifier.

Poll the Paseo daemon's Agent status via MCP API, and send notifications through configured channels when tasks complete, encounter errors, require user interaction, get stuck, recover from stuck, or during long-running heartbeats.

Supports parallel notification delivery across multiple providers. Built-in providers include console (with emoji), DingTalk bot, Feishu Webhook, and Feishu App. The plugin system (based on [notify](https://github.com/nikoksr/notify)) makes it easy to add 30+ services like Slack, Telegram, Discord, email, and more.

## Quick Install

```bash
go install github.com/winezer0/paseo-notifier/cmd/paseo-notifier@latest
```

The binary is installed to `$GOPATH/bin` (or `$GOBIN`). Then configure and start:

```bash
paseo-notifier --init          # Generate config file to program directory
# Edit paseo-notifier.yaml with your notification settings
paseo-notifier install         # Register as system service
paseo-notifier start           # Start service
```


## Architecture

### Data Flow

```
Daemon MCP API (127.0.0.1:6767)
       │
       ▼
  Agent Watcher (polls every 5s)
       │
       ├── list_agents                    → detect finished / error / stuck
       │   ├── get_agent_activity         → attach activity summary
       │   └── checkRunningAgents         → running status heartbeat
       ├── list_pending_permissions       → detect new permission requests
       └── get_agent_activity             → confirm stuck (async)
       │
       ▼
  Notifier.Notify(event)
       │
       ├── Console output (with emoji)
       ├── DingTalk bot
       ├── Feishu Webhook
       └── Feishu App
```

### Event Types

| Event | Trigger | Detection |
|:---|:---|:---|
| ✅ Task finished | `attentionReason: null → "finished"` | `list_agents` |
| ❌ Error occurred | `attentionReason: null → "error"` | `list_agents` |
| ⚠️ Interaction needed | New item in `list_pending_permissions` | Permission request list |
| 🔔 Stuck warning | `UpdatedAt` unchanged beyond `stuck_detect_timeout` | Before `get_agent_activity` confirmation |
| 🔔 Agent stuck confirmed | Last activity also exceeds timeout | `get_agent_activity` confirms |
| ℹ️ Still active | Agent found active after secondary check | `get_agent_activity` confirms |
| 🔄 Running status | Agent running beyond `running_status_interval` | `checkRunningAgents` |
| 🔔 Startup notification | Notifier initialized | First startup |
| 🔌 Disconnect/reconnect | MCP daemon connection lost/restored | Poll success/failure |

### Activity Summary

When **finished / error** events fire, `get_agent_activity` is automatically called to fetch the agent's recent execution timeline. The last 8 activity entries (tool calls, thinking steps, etc.) are appended to the notification content, so you know what the agent actually did — not just that it "completed".

### Stuck Detection

When a running agent's `UpdatedAt` field hasn't changed for longer than `stuck_detect_timeout` (default 120 seconds):

1. **Stuck warning**: A warning notification "Agent may be stuck (checking)" is sent immediately.
2. **Secondary check**: `get_agent_activity` is called to fetch the latest activity timeline.
3. **Verdict**:
   - Last activity also timed out → **Stuck confirmed** notification with idle duration, reason, and activity summary
   - Last activity is still within the timeout threshold → **Still active** notification with recent activity entries

After stuck detection, the auto-restart flow can be configured (`stuck_restart_delay` / `stuck_restart_retry`):
1. Send stuck notification after `stuck_detect_timeout`
2. After `stuck_restart_delay`, check if agent recovered on its own
3. If still stuck, auto-send a `continue` prompt to recover
4. Give up after `stuck_restart_retry` retries

> If `UpdatedAt` resumes normal updates during monitoring, the stuck state is automatically reset.

### Running Status Heartbeat

When an agent runs continuously beyond `running_status_interval` (default 5 minutes) without user interaction, a running status heartbeat notification is sent periodically. The notification includes agent info, running duration, and recent activity entries, keeping you informed of long-running agent progress.

### Notification Types

| Notification | Trigger | Channels |
|:---|:---|:---|
| Task completed | Agent task finished normally | All configured providers |
| Task error | Agent execution error | All configured providers |
| Permission request | Agent needs user confirmation | All configured providers |
| Stuck warning | `UpdatedAt` unchanged beyond `stuck_detect_timeout` | All configured providers |
| Stuck confirmed | Secondary check confirms agent is stuck | All configured providers |
| Still active | Secondary check finds agent working | All configured providers |
| Running status | Agent running beyond `running_status_interval` | All configured providers |
| Startup notification | Notifier initialized | All configured providers |
| Disconnect notification | MCP daemon connection lost | All configured providers |
| Reconnect notification | MCP daemon connection restored | All configured providers |

### Duplicate Notification Protection

- **finished / error**: Compares `(attentionReason, attentionTimestamp)`, skips if identical
- **Permission requests**: Tracks notified permission IDs
- **Stuck events**: Stuck warning, still active, and stuck confirmed each fire only once; auto-resets when `UpdatedAt` resumes
- **Running status**: At most one notification per `running_status_interval`
- **Disconnect/reconnect**: Clears state snapshot on reconnect to avoid backlog of duplicates
- **Archived agents**: Agents with `archivedAt` set are skipped

## Configuration

### Config File Search Order

1. Path specified by `--config`
2. `paseo-notifier.yaml` in the **program's directory**
3. Built-in default config (log output only)

> **Note**: Config and log files are only searched/created in the program's directory, not the user home directory.
> This ensures both foreground and SYSTEM service runs use the same config and logs, avoiding permission issues across different user accounts.

### Generate Config File

`--init` writes the complete default config file (with comments) to the program's directory:

```bash
# Write to <program-dir>/paseo-notifier.yaml
paseo-notifier --init

# Write to a custom path
paseo-notifier --config /path/to/custom.yaml --init
```

### Full Config Example

```yaml
# Monitor configuration
monitor:
  daemon_url: "http://127.0.0.1:6767/mcp/agents"
  interval: "5s"
  # Stuck detection timeout (Go time.Duration), 0s/false/empty = disabled, default 120s
  stuck_detect_timeout: 120s
  # Restart delay after stuck (Go time.Duration), 0s/false/empty = disabled, default 0s
  stuck_restart_delay: 0s
  stuck_restart_retry: 5
  # Running status heartbeat interval (Go time.Duration), 0s/false/empty = disabled, default 5m
  # Periodically sends running status for agents running beyond this interval
  running_status_interval: 5m

# Common settings (log, language)
common:
  # Log level: debug/info/warn/error (default info)
  log_level: "info"
  # Log file path (default: <program-dir>/paseo-notifier.log)
  # Leave empty to use default; auto-rotates when exceeding 10MB
  log_file: ""
  # Console log format, T=time L=level C=caller M=message (default TLM)
  # Set to false or off to disable console output
  log_console: "TLM"
  # Notification language
  # auto: auto-detect (Chinese system→zh, others→en)
  # zh:   Chinese
  # en:   English
  language: "auto"

# Notification provider configuration
# providers is an array; multiple providers can be configured simultaneously
# When no provider is configured, events are logged only
notifier:
  providers:
    # Console output (with emoji formatting)
    - type: "console"
      config:
        enable: true

    # DingTalk custom bot (Webhook + signing)
    # - type: "dingtalk"
    #   config:
    #     access_token: ""
    #     secret: ""

    # Feishu group bot (Webhook)
    # - type: "lark_webhook"
    #   config:
    #     webhook_url: "https://open.feishu.cn/open-apis/bot/v2/hook/your-webhook-id"

    # Feishu app (supports specifying receivers)
    # - type: "lark_app"
    #   config:
    #     app_id: "cli_your-app-id"
    #     app_secret: "your-app-secret"
    #     receivers:
    #       - type: chat_id
    #         value: "oc_your-chat-id"
```

`providers` is an array that supports multiple providers simultaneously. All enabled providers send notifications in parallel.

### Provider Types

| Type | Description | Config Parameters |
|:---|:---|:---|
| `console` | Console output (with emoji formatting) | `enable` (bool) |
| `dingtalk` | DingTalk custom bot (Webhook + signing) | `access_token`, `secret` |
| `lark_webhook` | Feishu group bot (Webhook) | `webhook_url` |
| `lark_app` | Feishu app (supports specifying receivers) | `app_id`, `app_secret`, `receivers[]` (type: open_id/user_id/union_id/email/chat_id, value) |

### Default Behavior

When no notification provider is configured, agent events are logged only (console and file), with no external notifications sent.


## Usage

### Build

```bash
# Full version (with system service management)
go build -o paseo-notifier.exe ./cmd/paseo-notifier

# Lightweight version (no service dependency, foreground only)
go build -tags noservice -o paseo-notifier.exe ./cmd/paseo-notifier
```

| Build | Service Commands | Use Case |
|-------|-----------------|----------|
| Default | install / uninstall / start / stop / restart | System service management, auto-start on boot |
| `-tags noservice` | Not available | Foreground only, smaller binary |

### Generate Config File

```bash
# Write to <program-dir>/paseo-notifier.yaml, edit and use directly
paseo-notifier --init
```

### Running

```bash
# Run in foreground
paseo-notifier

# Print version
paseo-notifier --version

# Specify config file
paseo-notifier --config /path/to/config.yaml

# Override log level (debug/info/warn/error)
paseo-notifier --ll debug

# Custom console format (T=time L=level C=caller F=func M=message)
paseo-notifier --lc "TLCM"

# Specify log file path
paseo-notifier --lf /path/to/custom.log
```

### View Logs

Log files are written to the program's directory by default:

```bash
# Windows
type <program-dir>\paseo-notifier.log

# Linux/macOS
tail -f <program-dir>/paseo-notifier.log
```

### System Service (Full Version)

Uses [kardianos/service](https://github.com/kardianos/service) for cross-platform service management.

#### Windows

Open PowerShell as Administrator:

```powershell
# 1. Generate config file (written to program directory)
.\paseo-notifier.exe --init

# 2. Edit .\paseo-notifier.yaml with your notification config

# 3. Register as Windows service and start
.\paseo-notifier.exe install
.\paseo-notifier.exe start

# 4. Check service status
Get-Service paseo-notifier

# 5. Stop / Restart
.\paseo-notifier.exe stop
.\paseo-notifier.exe restart

# 6. Uninstall service
.\paseo-notifier.exe stop
.\paseo-notifier.exe uninstall
```

After installation, a service named **paseo-notifier** appears in Windows Service Manager (`services.msc`), where you can set the startup type to "Automatic".

#### Linux (systemd)

```bash
# 1. Generate config file (written to program directory)
sudo ./paseo-notifier --init

# 2. Edit ./paseo-notifier.yaml with your notification config

# 3. Register systemd service
sudo ./paseo-notifier install

# 4. Start / Enable auto-start on boot
sudo ./paseo-notifier start
sudo systemctl enable paseo-notifier

# 5. Check status
sudo systemctl status paseo-notifier

# 6. Stop / Restart
sudo ./paseo-notifier stop
sudo ./paseo-notifier restart

# 7. Uninstall
sudo ./paseo-notifier stop
sudo ./paseo-notifier uninstall
```

#### macOS (Launchd)

```bash
# Register Launchd service
sudo ./paseo-notifier install

# Start / Stop / Uninstall
sudo ./paseo-notifier start
sudo ./paseo-notifier stop
sudo ./paseo-notifier uninstall
```

## Development

### Adding Custom Notification Providers

The `message/` package implements a provider registration mechanism. Adding a new provider takes just two steps:

1. Create a new file in the `message/` package (e.g., `provider_slack.go`)
2. Register the factory via `init()` calling `RegisterProvider`

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

No core code changes needed — `init()` handles registration automatically; `notifier.go` looks up and invokes the factory via the registry.

The underlying Notify library supports 30+ notification services, including Slack, Telegram, Discord, email, and more. See the full list at [nikoksr/notify](https://github.com/nikoksr/notify).

## Dependencies

- [nikoksr/notify](https://github.com/nikoksr/notify) — Multi-channel notification library
- [kardianos/service](https://github.com/kardianos/service) — Cross-platform system service management
- [go-flags](https://github.com/jessevdk/go-flags) — CLI argument parsing
- [yaml.v3](https://gopkg.in/yaml.v3) — YAML config parsing
