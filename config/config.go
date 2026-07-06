package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AppName 应用名称
const AppName = "paseo-notifier"
const appConfig = AppName + ".yaml"
const appLogPath = AppName + ".log"
const Version = "0.0.4"

// MonitorConfig 监控相关配置
type MonitorConfig struct {
	DaemonURL string `yaml:"daemon_url"`
	Interval  string `yaml:"interval"`
}

// DingTalkConfig 钉钉机器人配置（webhook 模式 + 加签）
type DingTalkConfig struct {
	AccessToken string `yaml:"access_token"`
	Secret      string `yaml:"secret"`
}

// LarkWebhookConfig 飞书 Webhook 机器人配置
type LarkWebhookConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

// LarkAppConfig 飞书自应用配置
type LarkAppConfig struct {
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
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

// CommonConfig 通用配置（日志、语言等）
type CommonConfig struct {
	LogFormat  string `yaml:"log_format"`
	LogPath    string `yaml:"log_path"`
	LogConsole *bool  `yaml:"log_console"`
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
		slog.Warn("invalid monitor interval, falling back to 5s", "value", m.Interval, "err", err)
		return 5 * time.Second
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
	t := true
	return &Config{
		Monitor: MonitorConfig{
			DaemonURL: "http://127.0.0.1:6767/mcp/agents",
			Interval:  "5s",
		},
		Notifier: NotifierConfig{
			Providers: nil,
		},
		Common: CommonConfig{
			LogFormat:  "text",
			LogPath:    "",
			LogConsole: &t,
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
