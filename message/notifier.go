package message

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nikoksr/notify"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
)

// noopNotifier 不执行任何通知操作
type noopNotifier struct{}

func (n *noopNotifier) Notify(event agentwatcher.AgentEvent) error {
	slog.Debug("event received but no notifier configured",
		"type", event.Type,
		"agent", event.Agent.ShortID)
	return nil
}

// BuildNotifier 根据配置构建通知器，支持多个供应商同时使用
// 没有配置有效供应商时返回 noopNotifier，日志由独立的 logger 处理
func BuildNotifier(cfg *config.Config) agentwatcher.Notifier {
	providers := cfg.Notifier.Providers

	if len(providers) == 0 {
		slog.Warn("no notification providers configured, events will be logged only")
		return &noopNotifier{}
	}

	var services []notify.Notifier
	for _, p := range providers {
		factory, ok := GetProvider(p.Type)
		if !ok {
			slog.Warn("unknown provider type, skipping", "type", p.Type)
			continue
		}

		svc, err := factory(p.Config)
		if err != nil {
			slog.Warn("failed to build provider, skipping", "type", p.Type, "err", err)
			continue
		}

		services = append(services, svc)
		slog.Info("notification provider enabled", "type", p.Type)
	}

	if len(services) == 0 {
		slog.Warn("no valid notification provider configured, events will be logged only")
		return &noopNotifier{}
	}

	notify.UseServices(services...)
	return &NotifyNotifier{}
}

// SendStartupNotification 发送启动通知
func SendStartupNotification(notifier agentwatcher.Notifier) {
	msg := getMessages(currentLang)
	if _, ok := notifier.(*NotifyNotifier); ok {
		subject := fmt.Sprintf(msg.SubjectStartup, config.AppName)
		content := msg.StartupContent
		slog.Info("sending startup notification")
		if err := notify.Send(context.Background(), subject, content); err != nil {
			slog.Warn("startup notification failed", "err", err)
		} else {
			slog.Info("startup notification sent")
		}
	} else {
		slog.Info("startup notification skipped (no external notifier configured)")
	}
}

// BuildSystemNotify 构建系统事件通知（断连/重连）
func BuildSystemNotify(disconnected bool, daemonURL string) (subject, content string) {
	msg := getMessages(currentLang)
	if disconnected {
		subject = msg.SubjectDisconnect
	} else {
		subject = msg.SubjectReconnect
	}
	content = fmt.Sprintf("%s: %s\n%s: %s\n%s: %s",
		msg.FieldDaemon, daemonURL,
		msg.FieldTime, time.Now().Format("2006-01-02 15:04:05"),
		msg.FieldStatus, func() string {
			if disconnected {
				return msg.DisconnectContent
			}
			return msg.ReconnectContent
		}())
	return
}
