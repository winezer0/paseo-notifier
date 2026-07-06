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
	"fmt"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
	"github.com/winezer0/paseo-notifier/message"
)

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

	if opts.Cleanup != "" {
		cfg := initConfigAndLogger(opts)
		defer logging.Sync()

		durStr := opts.Cleanup
		retention, err := time.ParseDuration(durStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid cleanup duration %q: %v\n", durStr, err)
			os.Exit(1)
		}

		w := agentwatcher.NewWatcher(cfg.Monitor, &message.NoopNotifier{}, "")
		defer w.Stop()

		count, err := w.CleanupArchivedAgents(retention)
		if err != nil {
			logging.Errorf("cleanup failed: %v", err)
			os.Exit(1)
		}
		if retention <= 0 {
			logging.Infof("cleanup completed: %d archived agents killed", count)
		} else {
			logging.Infof("cleanup completed: %d archived agents (older than %s) killed", count, retention)
		}
		return
	}

	svcConfig := &service.Config{
		Name:        config.AppName,
		DisplayName: config.AppName,
		Description: "Monitor Paseo Agent status and send notifications via configured channels",
	}

	// 前台运行 / 服务运行
	if action == "" || action == "run" {
		cfg := initConfigAndLogger(opts)
		defer logging.Sync()

		prg := &program{cfg: cfg}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			logging.Errorf("create service failed: %v", err)
			os.Exit(1)
		}

		if err := s.Run(); err != nil {
			logging.Errorf("service run failed: %v", err)
			os.Exit(1)
		}
	} else {
		// 服务管理命令（install / uninstall / start / stop / restart）
		if _, valid := serviceActions[action]; !valid {
			fmt.Fprintf(os.Stderr, "unknown command: %q\n", action)
			fmt.Fprintf(os.Stderr, "available commands: install, uninstall, start, stop, restart, run\n")
			os.Exit(1)
		}

		consoleCfg := logging.NewLogConfig("info", "", "T L C M")
		if err := logging.InitDefaultLogger(consoleCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer logging.Sync()

		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			logging.Errorf("create service failed: %v", err)
			os.Exit(1)
		}

		if err := service.Control(s, action); err != nil {
			logging.Errorf("service control failed: %v", err)
			os.Exit(1)
		}
		logging.Infof("service %s completed successfully", action)
	}
}

// initConfigAndLogger 加载配置并初始化日志器，失败时直接 exit
func initConfigAndLogger(opts *cliOptions) *config.Config {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config failed: %v\n", err)
		os.Exit(1)
	}

	logCfg := mergeLogConfig(opts, cfg)
	if err := logging.InitDefaultLogger(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	return cfg
}
