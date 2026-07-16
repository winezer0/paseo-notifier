package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/winezer0/paseo-notifier/logging"
	"gopkg.in/yaml.v3"
)

// AppName 应用名称
const AppName = "paseo-notifier"
const appConfig = AppName + ".yaml"
const appLogPath = AppName + ".log"
const Version = "0.1.4"

// MonitorConfig 监控相关配置
type MonitorConfig struct {
	DaemonURL               string          `yaml:"daemon_url"`
	Interval                string          `yaml:"interval"`
	StuckDetectTimeout      string          `yaml:"stuck_detect_timeout"`
	StuckRestartDelay       string          `yaml:"stuck_restart_delay"`
	StuckRestartRetry       int             `yaml:"stuck_restart_retry"`
	RunningStatusInterval   string          `yaml:"running_status_interval"`
	SubagentRunningInterval string          `yaml:"subagent_running_interval"` // subagent 持续运行通知间隔，默认 3m
	AutoContinueKeyword     bool            `yaml:"auto_continue_keyword"`     // 匹配关键字自动继续（任务完成时检测"继续"/"continue"关键词）
	AutoContinueSubagent    bool            `yaml:"auto_continue_subagent"`    // 子任务完成后自动继续（所有subagent完成且主agent空闲时触发）
	NotifyMinDuration       string          `yaml:"notify_min_duration"`       // 最短任务通知时长，短于此时长完成的任务不发送通知
	Events                  map[string]bool `yaml:"events,omitempty"`          // 事件开关映射，未列出的事件默认启用
}

// ProviderItem 单个通知供应商配置项
type ProviderItem struct {
	Type string `yaml:"type"`

	// Config 是供应商特有的配置，由对应工厂函数解码为具体结构体
	Config yaml.Node `yaml:"config"`
}

// NotifierConfig 通知器配置
type NotifierConfig struct {
	Providers []ProviderItem `yaml:"providers"`
}

// CommonConfig 通用配置（日志、语言、dry-run 等）
type CommonConfig struct {
	LogLevel   string `yaml:"log_level"`
	LogFile    string `yaml:"log_file"`
	LogConsole string `yaml:"log_console"`
	Language   string `yaml:"language"`
}

// Config 总配置
type Config struct {
	Monitor  MonitorConfig  `yaml:"monitor"`
	Notifier NotifierConfig `yaml:"notifier"`
	Common   CommonConfig   `yaml:"common"`
}

// IntervalDuration 解析间隔字符串为 Go 时间间隔
// 解析失败时返回 5s
func (m *MonitorConfig) IntervalDuration() time.Duration {
	d, err := time.ParseDuration(m.Interval)
	if err != nil {
		logging.Warnf("invalid monitor interval, falling back to 5s value=%s err=%v", m.Interval, err)
		return 5 * time.Second
	}
	return d
}

// StuckDetectDuration 解析卡死检测超时，0 表示禁用
func (m *MonitorConfig) StuckDetectDuration() time.Duration {
	d := parseDuration(m.StuckDetectTimeout, "stuck_detect_timeout")
	if d > 0 {
		return d
	}
	return 120 * time.Second
}

// StuckRestartDuration 解析卡死重启延迟，0 表示禁用
func (m *MonitorConfig) StuckRestartDuration() time.Duration {
	return parseDuration(m.StuckRestartDelay, "stuck_restart_delay")
}

// RunningStatusIntervalDuration 解析运行中状态心跳通知间隔，0 表示禁用
func (m *MonitorConfig) RunningStatusIntervalDuration() time.Duration {
	d := parseDuration(m.RunningStatusInterval, "running_status_interval")
	if d > 0 {
		return d
	}
	return 5 * time.Minute
}

// SubagentRunningIntervalDuration 解析 subagent 持续运行通知间隔，0 表示禁用
func (m *MonitorConfig) SubagentRunningIntervalDuration() time.Duration {
	d := parseDuration(m.SubagentRunningInterval, "subagent_running_interval")
	if d > 0 {
		return d
	}
	return 3 * time.Minute
}

// NotifyMinDurationDuration 解析最短任务通知时长，短于此时长完成的任务不发送通知
// 0 表示不抑制，默认 30s
func (m *MonitorConfig) NotifyMinDurationDuration() time.Duration {
	if m.NotifyMinDuration == "" {
		return 30 * time.Second
	}
	d := parseDuration(m.NotifyMinDuration, "notify_min_duration")
	return d
}

// parseDuration 解析时间字符串，"false"/空/0 等均返回 0（禁用）
func parseDuration(raw, field string) time.Duration {
	if raw == "" || raw == "false" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logging.Warnf("invalid %s, falling back to 0 value=%s err=%v", field, raw, err)
		return 0
	}
	if d < 0 {
		logging.Warnf("negative %s, falling back to 0 value=%s", field, raw)
		return 0
	}
	return d
}

// DefaultLogPath 返回程序所在目录下的默认日志路径
func DefaultLogPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), appLogPath)
	}
	return ""
}

// DefaultConfig 返回带有内置默认值的配置
func DefaultConfig() *Config {
	return &Config{
		Monitor: MonitorConfig{
			DaemonURL:               "http://127.0.0.1:6767",
			Interval:                "5s",
			StuckDetectTimeout:      "120s",
			StuckRestartDelay:       "0s",
			StuckRestartRetry:       5,
			RunningStatusInterval:   "5m",
			SubagentRunningInterval: "3m",
			AutoContinueKeyword:     false,
			AutoContinueSubagent:    false,
			NotifyMinDuration:       "30s",
		},
		Notifier: NotifierConfig{
			Providers: nil,
		},
		Common: CommonConfig{
			LogFile:    "",
			LogLevel:   "info",
			LogConsole: "LM",
		},
	}
}

// AppDir 返回程序可执行文件所在目录
func AppDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// AppConfigPath 返回程序所在目录下的默认配置文件路径
func AppConfigPath() string {
	return filepath.Join(AppDir(), appConfig)
}
