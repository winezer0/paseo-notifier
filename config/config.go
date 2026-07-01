package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AppName 应用名称
const AppName = "paseo-notifier"
const appConfig = AppName + ".yaml"
const appLogPath = AppName + ".log"
const Version = "0.0.2"

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

// Config 总配置
type Config struct {
	Monitor    MonitorConfig  `yaml:"monitor"`
	Notifier   NotifierConfig `yaml:"notifier"`
	LogFormat  string         `yaml:"log_format"`
	LogPath    string         `yaml:"log_path"`
	LogConsole *bool          `yaml:"log_console"`
	Language   string         `yaml:"language"`
}

// IntervalDuration 解析轮询间隔
func (m *MonitorConfig) IntervalDuration() time.Duration {
	d, err := time.ParseDuration(m.Interval)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

// DefaultLogPath 返回默认日志路径（程序所在目录）
func DefaultLogPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), appLogPath)
	}
	return ""
}

// DefaultConfig 返回默认配置
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
		LogFormat:  "text",
		LogPath:    "",
		LogConsole: &t,
	}
}

// AppDir 返回程序所在目录
func AppDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// Load searches for config in program directory.
// Returns default config if none found.
func Load(path string) (*Config, error) {
	if path != "" {
		return loadFile(path)
	}

	full := filepath.Join(AppDir(), appConfig)
	if cfg, err := loadFile(full); err == nil {
		return cfg, nil
	}

	return DefaultConfig(), nil
}

// loadFile 从指定路径加载配置文件
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	if cfg.Monitor.DaemonURL == "" {
		cfg.Monitor.DaemonURL = "http://127.0.0.1:6767/mcp/agents"
	}
	if cfg.Monitor.Interval == "" {
		cfg.Monitor.Interval = "5s"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}
	if cfg.LogPath == "" {
		cfg.LogPath = DefaultLogPath()
	}
	if cfg.LogConsole == nil {
		t := true
		cfg.LogConsole = &t
	}

	return cfg, nil
}

// AppConfigPath 返回程序所在目录下的配置文件路径
func AppConfigPath() string {
	return filepath.Join(AppDir(), appConfig)
}
