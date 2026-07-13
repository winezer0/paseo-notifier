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

// BuildNotifier 根据配置构建通知器。
// 所有供应商（包括 console）都通过注册工厂构建，由 notify.UseServices 统一管理。
// 未配置任何供应商时返回 NoopNotifier（仅日志输出不发送通知）。
// 配置了 events 开关时，返回包装后的 EventFilterNotifier 按事件类型过滤。
func BuildNotifier(cfg *config.Config) agentwatcher.Notifier {
	providers := cfg.Notifier.Providers
	if len(providers) == 0 {
		logging.Info("no notifier configured, events will be logged only")
		return &NoopNotifier{}
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
		logging.Info("no valid providers, events will be logged only")
		return &NoopNotifier{}
	}

	notify.UseServices(services...)
	return WrapEventFilter(&NotifyNotifier{}, cfg.Monitor.Events)
}

// SendStartupNotification 发送启动通知，仅当通知器非 NoopNotifier 时实际发送
func SendStartupNotification(notifier agentwatcher.Notifier, events map[string]bool) {
	if !IsEventEnabled(events, agentwatcher.EventStartup) {
		logging.Info("startup notification disabled by config")
		return
	}
	msg := getMessages(currentLang)
	if _, ok := notifier.(*NoopNotifier); ok {
		logging.Info("startup notification skipped (no external notifier configured)")
		return
	}
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
