package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/winezer0/paseo-notifier/config"
	"github.com/winezer0/paseo-notifier/logging"
)

// cliOptions 命令行参数
type cliOptions struct {
	Config     string `short:"c" long:"config" description:"config file path" value-name:"FILE"`
	Init       bool   `short:"i" long:"init" description:"print default config and exit"`
	Version    bool   `short:"v" long:"version" description:"print version and exit"`
	Cleanup    string `short:"C" long:"cleanup" optional:"true" optional-value:"12h" description:"cleanup archived agents before given duration (e.g. 12h, 0h for all); default: 12h" value-name:"DURATION"`
	LogFile    string `long:"lf" description:"Log file path"`
	LogLevel   string `long:"ll" description:"Log level: allowed debug/info/warn/error"`
	LogConsole string `long:"lc" description:"Log format for console, supported T(time),L(level),C(caller),F(func),M(Msg), Turn off when empty or (off)"`
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
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			printServiceHelp()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "parse flags error: %v\n", err)
		os.Exit(1)
	}

	if len(args) > 0 {
		action = args[0]
	}

	return opts, action
}

// printServiceHelp 打印扩展帮助信息，包括服务管理命令和配置文件搜索顺序
// go-flags 已在 ErrHelp 时自动输出帮助文本，此处仅追加额外信息
func printServiceHelp() {
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

// mergeLogConfig 合并日志配置，CLI 参数显式设置时使用 CLI，否则使用配置文件
func mergeLogConfig(opts *cliOptions, cfg *config.Config) logging.LogConfig {
	logFile := config.DefaultLogPath()
	logLevel := "info"
	consoleFormat := "TLCM"
	// CLI 模式：只要用户显式传了任何日志参数，全部从 CLI 获取
	if opts.LogFile != "" || opts.LogLevel != "" || opts.LogConsole != "" {
		if opts.LogFile != "" {
			logFile = opts.LogFile
		}
		if opts.LogLevel != "" {
			logLevel = opts.LogLevel
		}
		if opts.LogConsole != "" {
			consoleFormat = opts.LogConsole
		}
	} else {
		// 服务模式：无 CLI 日志参数，从配置文件获取
		if cfg.Common.LogFile != "" {
			logFile = cfg.Common.LogFile
		}
		if cfg.Common.LogLevel != "" {
			logLevel = cfg.Common.LogLevel
		}
		if cfg.Common.LogConsole != "" {
			consoleFormat = cfg.Common.LogConsole
		}
	}
	return logging.NewLogConfig(logLevel, logFile, consoleFormat)
}
