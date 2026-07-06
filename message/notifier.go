package message

import (
	"context"
	"fmt"
	"time"

	"github.com/nikoksr/notify"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
)

// noopNotifier 不执行任何通知操作
type noopNotifier struct{}

// Notify 实现了 agentwatcher.Notifier，仅记录日志不发送实际通知
func (n *noopNotifier) Notify(event agentwatcher.AgentEvent) error {
	logging.Debugf("event received but no notifier configured type=%s agent=%s", event.Type, event.Agent.ShortID)
	return nil
}

// BuildNotifier 根据配置构建通知器，支持同时使用多个供应商
// 未配置有效供应商时返回 noopNotifier
func BuildNotifier(cfg *config.Config) agentwatcher.Notifier {
	providers := cfg.Notifier.Providers

	if len(providers) == 0 {
		logging.Warn("no notification providers configured, events will be logged only")
		return &noopNotifier{}
	}

	var services []notify.Notifier
	for _, p := range providers {
		factory, ok := GetProvider(p.Type)
		if !ok {
			logging.Warnf("unknown provider type, skipping type=%s", p.Type)
			continue
		}

		svc, err := factory(p.Config)
		if err != nil {
			logging.Warnf("failed to build provider, skipping type=%s err=%v", p.Type, err)
			continue
		}

		services = append(services, svc)
		logging.Infof("notification provider enabled type=%s", p.Type)
	}

	if len(services) == 0 {
		logging.Warn("no valid notification provider configured, events will be logged only")
		return &noopNotifier{}
	}

	notify.UseServices(services...)
	return &NotifyNotifier{}
}

// SendStartupNotification 发送启动通知，仅当通知器为 NotifyNotifier 时实际发送
func SendStartupNotification(notifier agentwatcher.Notifier) {
	msg := getMessages(currentLang)
	if _, ok := notifier.(*NotifyNotifier); ok {
		subject := fmt.Sprintf(msg.SubjectStartup, config.AppName)
		content := msg.StartupContent
		fmt.Println()
		fmt.Println("=== Notification ===")
		fmt.Println("Subject:", subject)
		fmt.Println(content)
		fmt.Println("====================")
		fmt.Println()
		if err := notify.Send(context.Background(), subject, content); err != nil {
			logging.Warnf("startup notification failed: %v", err)
		} else {
			logging.Info("startup notification sent")
		}
	} else {
		logging.Info("startup notification skipped (no external notifier configured)")
	}
}

// BuildSystemNotify 构建系统事件通知的主题和内容（断连/重连）
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
