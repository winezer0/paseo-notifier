package main

import (
	"context"
	"fmt"

	"github.com/kardianos/service"
	"github.com/nikoksr/notify"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
	"github.com/winezer0/paseo-notifier/message"
)

// serviceActions 服务管理命令列表
var serviceActions = map[string]string{
	"install":   "register as system service",
	"uninstall": "uninstall system service",
	"start":     "start system service",
	"stop":      "stop system service",
	"restart":   "restart system service",
}

// program 实现 service.Interface，封装通知监控生命周期
// 注意：p.cfg 由 main() 在 s.Run() 之前设置，Start() 中不应为 nil
type program struct {
	cfg      *config.Config
	watcher  *agentwatcher.Watcher
	notifier agentwatcher.Notifier
}

// Start 实现 service.Interface.Start，构建通知器、启动监控
func (p *program) Start(s service.Service) error {
	providerTypes := make([]string, len(p.cfg.Notifier.Providers))
	for i, pr := range p.cfg.Notifier.Providers {
		providerTypes[i] = pr.Type
	}
	logging.Infof("config loaded daemon=%s interval=%s notifier_providers=%v language=%s version=%s",
		p.cfg.Monitor.DaemonURL, p.cfg.Monitor.Interval,
		providerTypes, p.cfg.Common.Language, config.Version)

	message.SetLang(message.ResolveLang(p.cfg.Common.Language))

	p.notifier = message.BuildNotifier(p.cfg)

	p.watcher = agentwatcher.NewWatcher(
		p.cfg.Monitor,
		p.notifier,
		message.BuildContinuePrompt(),
		message.BuildStuckContinuePrompt(),
	)
	p.watcher.SetEvents(p.cfg.Monitor.Events)

	if _, ok := p.notifier.(*message.NotifyNotifier); ok {
		p.watcher.SetSystemNotifier(func(disconnected bool, daemonURL string) {
			subject, content := message.BuildSystemNotify(disconnected, daemonURL)
			fmt.Println()
			fmt.Println("=== System Notification ===")
			fmt.Println("Subject:", subject)
			fmt.Println(content)
			fmt.Println("===========================")
			fmt.Println()
			if err := notify.Send(context.Background(), subject, content); err != nil {
				logging.Errorf("system notify failed: %v", err)
			}
		})
	}

	message.SendStartupNotification(p.notifier, p.cfg.Monitor.Events)

	p.watcher.Start()

	return nil
}

// Stop 实现 service.Interface.Stop，停止监听器并关闭日志
func (p *program) Stop(s service.Service) error {
	if p.watcher == nil {
		return nil
	}

	logging.Info("shutting down...")
	p.watcher.Stop()
	_ = logging.Sync() // best-effort，Windows 下 stdout 句柄可能已关闭
	logging.Info("stopped")
	return nil
}
