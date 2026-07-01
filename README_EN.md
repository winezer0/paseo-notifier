**English** | [中文](./README.md)

# paseo-notifier

Paseo Agent status notifier.

Both the desktop (Windows) and mobile (Android) versions of Paseo v0.1.102 have issues with sound notifications — messages cannot be effectively alerted after completion.

After trying various approaches without success, the lack of notifications means being unaware of task running status, which significantly impacts efficiency.

The solution: poll the Paseo daemon's Agent status via MCP API, and send notifications through configured channels when tasks complete, encounter errors, or require user interaction.

Built on the [notify](https://github.com/nikoksr/notify) library, it supports multiple notification methods. Currently supports DingTalk, Feishu (Lark) Webhook, and Feishu App notifications. See below for registering additional provider types.

## Architecture

```
paseo-notifier/
├── agentwatcher/                   # Agent status monitoring
│   ├── watcher.go                     MCP API polling + state diff engine
│   └── watcher_test.go
├── cmd/paseo-notifier/             # CLI entry point
│   └── main.go                        program implements service.Interface
│                                      install/uninstall/start/stop/restart
│                                      build tag: -tags noservice for no-service build
├── config/                         # Configuration management
│   ├── config.go                      YAML config loading
│   └── default.go                     --init writes default config
├── embeds/                         # Embedded resources
│   ├── config.yaml                    Fully commented default config template
│   └── embeds.go                      //go:embed declaration
├── logger/                         # Logging
│   ├── init.go                        InitLogger initialization
│   └── filelog.go                     File logging + 10MB auto-rotation
├── message/                        # Notification messages
│   ├── message.go                     Event → formatted message text
│   ├── i18n.go                        Bilingual text + language detection
│   ├── i18n_windows.go                Windows system language detection
│   ├── i18n_unix.go                   Unix system language detection
│   ├── notifier.go                    Notifier builder + startup notification
│   ├── notifynotifier.go              NotifyNotifier adapter
│   ├── provider.go                    Provider registry
│   ├── provider_dingtalk.go           DingTalk Webhook
│   ├── provider_lark_webhook.go       Feishu Webhook
│   ├── provider_lark_app.go           Feishu App
│   ├── notifier_test.go
│   └── provider_test.go
├── README.md
└── go.mod / go.sum
```

### Data Flow

```
Daemon MCP API (127.0.0.1:6767)
       │
       ▼
  Agent Watcher (polls every 5s)
       │
       ├── list_agents                    → detect finished / error
       └── list_pending_permissions       → detect new permission requests
       │
       ▼
  Notifier.Notify(event)
       │
       ▼
  notify.UseServices(svc...)
       │
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

### Duplicate Notification Protection

- **finished / error**: Compares `(attentionReason, attentionTimestamp)`, skips if identical
- **Permission requests**: Tracks notified permission IDs
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
monitor:
  daemon_url: "http://127.0.0.1:6767/mcp/agents"
  interval: "5s"

log_format: "text"

# Log file path (default: <program-dir>/paseo-notifier.log)
# Leave empty to use default; auto-rotates when exceeding 10MB
log_path: ""

# Also output logs to console (default true)
log_console: true

# Notification message language (default auto, auto-detects system language)
# Options: auto / zh / en
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

`providers` is an array that supports multiple providers simultaneously. All enabled providers send notifications in parallel.

### Provider Types

| Type | Description | Config Parameters |
|:---|:---|:---|
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
