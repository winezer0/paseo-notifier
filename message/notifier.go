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
func (n *noopNotifier) Notify(ctx context.Context, event agentwatcher.AgentEvent) error {
	logging.Debugf("event received but no notifier configured type=%s agent=%s", event.Type, event.Agent.ShortID)
	return nil
}

// BuildNotifier 根据配置构建通知器，支持同时使用多个供应商。
// 未配置任何供应商时默认使用 consoleNotifier，保证用户始终能看到输出。
// ponytail: console provider 从真实 provider 列表剥离，避免 notify.UseServices 双重复制
func BuildNotifier(cfg *config.Config) agentwatcher.Notifier {
	providers := cfg.Notifier.Providers

	// 检查是否有 console provider，并从真实 provider 列表中剥离
	var realProviders []config.ProviderItem
	for _, p := range providers {
		if p.Type == "console" {
			continue
		}
		realProviders = append(realProviders, p)
	}

	// 没有真实 provider → 默认使用控制台输出
	if len(realProviders) == 0 {
		logging.Info("console notification enabled")
		return &consoleNotifier{}
	}

	// 构建真实的通知服务
	var services []notify.Notifier
	for _, p := range realProviders {
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
		logging.Info("console notification enabled (all configured providers invalid)")
		return &consoleNotifier{}
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
