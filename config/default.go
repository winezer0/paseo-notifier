package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/winezer0/paseo-notifier/embeds"
	"gopkg.in/yaml.v3"
)

// WriteDefaultConfig 将内嵌的默认配置 YAML 写入指定路径
// 如果 cfgPath 为空，则使用程序目录下的默认路径
func WriteDefaultConfig(cfgPath string) error {
	if cfgPath == "" {
		cfgPath = AppConfigPath()
	}
	if cfgPath == "" {
		return fmt.Errorf("cannot determine config path")
	}

	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}

	if err := os.WriteFile(cfgPath, embeds.DefaultConfigYAML, 0644); err != nil {
		return fmt.Errorf("write config to %s: %w", cfgPath, err)
	}

	fmt.Printf("config file written to: %s\n", cfgPath)
	return nil
}

// Load searches for config in program directory.
// Returns default config if none found.
func Load(path string) (*Config, error) {
	if path == "" {
		path = AppConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 没有配置文件时，返回内置默认配置（仅日志输出，无通知供应商）
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	// 1. 获取默认配置模板
	cfg := DefaultConfig()

	// 2. 解析 YAML 覆盖默认值
	// 注意：标准 yaml.Unmarshal 只会覆盖 YAML 中存在的字段，不会清空 cfg 中已有的默认值
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	// 3. 删除这里所有的 if cfg.XXX == "" 判断
	// 因为 cfg 已经从 DefaultConfig() 继承了默认值，
	// 且 Unmarshal 只会覆盖非空字段，所以这里不需要再次检查。

	return cfg, nil
}
