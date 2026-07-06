// paseo-notifier — 独立运行的 Paseo Agent 状态通知器。
//
// 通过 Paseo 守护进程的 MCP API 轮询 Agent 状态，在以下场景
// 通过配置的通知渠道发送通知：
//
//   - Agent 任务完成
//   - Agent 运行出错
//   - Agent 需要用户交互（权限请求）
//
// 支持以系统服务模式运行，也可直接前台运行。
// 支持 install / uninstall / start / stop / restart 服务管理命令。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/jessevdk/go-flags"
	"github.com/kardianos/service"
	"github.com/nikoksr/notify"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logger"
	"github.com/winezer0/paseo-notifier/message"
)

// cliOptions 命令行参数
type cliOptions struct {
	Config  string `short:"c" long:"config" description:"config file path" value-name:"FILE"`
	Init    bool   `short:"i" long:"init" description:"print default config and exit"`
	Version bool   `short:"v" long:"version" description:"print version and exit"`
}

// serviceActions 服务管理命令列表
var serviceActions = map[string]string{
	"install":   "register as system service",
	"uninstall": "uninstall system service",
	"start":     "start system service",
	"stop":      "stop system service",
	"restart":   "restart system service",
}

// printExtraHelp 打印扩展帮助信息，包括服务管理命令和配置文件搜索顺序
func printExtraHelp(parser *flags.Parser) {
	parser.WriteHelp(os.Stdout)
	fmt.Println()
	fmt.Println("Service management commands (first argument):")
	for name, desc := range serviceActions {
		fmt.Printf("  %-12s %s\n", name, desc)
	}
	fmt.Println()
	fmt.Println("Config file search order:")
	fmt.Println("  1. Path specified by --config")
	fmt.Printf("  2. %s (program directory)\n", config.AppConfigPath())
	fmt.Println("  3. Built-in default config (log output only)")
}

// parseCLI 解析命令行参数，返回选项结构和可选的服务管理命令
func parseCLI() (opts *cliOptions, action string) {
	opts = &cliOptions{}
	parser := flags.NewParser(opts, flags.Default)
	parser.Name = config.AppName
	parser.ShortDescription = "Paseo Agent status notifier"
	parser.LongDescription = "Monitors Paseo agents via MCP API and sends notifications on task completion, errors, or permission requests."

	args, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			printExtraHelp(parser)
			os.Exit(0)
		}
		os.Exit(1)
	}

	if len(args) > 0 {
		action = args[0]
	}

	return opts, action
}

func main() {
	opts, action := parseCLI()

	if opts.Version {
		fmt.Println(config.Version)
		return
	}

	if opts.Init {
		if err := config.WriteDefaultConfig(opts.Config); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	svcConfig := &service.Config{
		Name:        config.AppName,
		DisplayName: config.AppName,
		Description: "Monitor Paseo Agent status and send notifications via configured channels",
	}

	if action == "" || action == "run" {
		// 前台/服务运行模式
		cfg, err := config.Load(opts.Config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		prg := &program{cfg: cfg}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		logger, err := s.Logger(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if err := s.Run(); err != nil {
			logger.Error(err)
			os.Exit(1)
		}
		return
	}

	// 服务管理命令（install / uninstall / start / stop / restart）
	if _, valid := serviceActions[action]; !valid {
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", action)
		fmt.Fprintf(os.Stderr, "available commands: install, uninstall, start, stop, restart, run\n")
		os.Exit(1)
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := service.Control(s, action); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service %s completed successfully\n", action)
}

// program 实现 service.Interface，封装通知监控生命周期
type program struct {
	cfg      *config.Config
	watcher  *agentwatcher.Watcher
	notifier agentwatcher.Notifier
	mu       sync.Mutex
	started  bool
}

// Start 实现 service.Interface.Start，加载配置、初始化日志、构建通知器、启动监控
func (p *program) Start(s service.Service) error {
	if p.cfg == nil {
		cfg, err := config.Load("")
		if err != nil {
			slog.Error("failed to load config", "err", err)
			return fmt.Errorf("load config: %w", err)
		}
		p.cfg = cfg
		slog.Info("config file loaded",
			"path", config.AppConfigPath(),
			"daemon", p.cfg.Monitor.DaemonURL,
			"interval", p.cfg.Monitor.Interval)
	}

	logConsole := p.cfg.Common.LogConsole != nil && *p.cfg.Common.LogConsole
	if err := logger.InitLogger(p.cfg.Common.LogPath, p.cfg.Common.LogFormat, logConsole, slog.LevelInfo); err != nil {
		slog.Error("init logger with config failed", "err", err)
	}

	providerTypes := make([]string, len(p.cfg.Notifier.Providers))
	for i, pr := range p.cfg.Notifier.Providers {
		providerTypes[i] = pr.Type
	}
	slog.Info("config loaded",
		"daemon", p.cfg.Monitor.DaemonURL,
		"interval", p.cfg.Monitor.Interval,
		"notifier_providers", providerTypes,
		"language", p.cfg.Common.Language,
		"version", config.Version)

	message.SetLang(message.ResolveLang(p.cfg.Common.Language))

	p.notifier = message.BuildNotifier(p.cfg)

	p.watcher = agentwatcher.NewWatcher(
		p.cfg.Monitor.DaemonURL,
		p.cfg.Monitor.IntervalDuration(),
		p.notifier,
	)

	if _, ok := p.notifier.(*message.NotifyNotifier); ok {
		p.watcher.SetSystemNotifier(func(disconnected bool, daemonURL string) {
			subject, content := message.BuildSystemNotify(disconnected, daemonURL)
			if err := notify.Send(context.Background(), subject, content); err != nil {
				slog.Error("system notify failed", "err", err)
			}
		})
	}

	message.SendStartupNotification(p.notifier)

	p.watcher.Start()

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	return nil
}

// Stop 实现 service.Interface.Stop，停止监听器并关闭日志
func (p *program) Stop(s service.Service) error {
	p.mu.Lock()
	started := p.started
	p.mu.Unlock()

	if !started || p.watcher == nil {
		return nil
	}

	slog.Info("shutting down...")
	p.watcher.Stop()
	logger.CloseLogger()
	slog.Info("stopped")
	return nil
}
