package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/winezer0/paseo-notifier/embeds"
	"gopkg.in/yaml.v3"
)

// WriteDefaultConfig 将默认配置写入指定路径。
// 若目标文件不存在 → 写入内嵌模板（含完整注释）。
// 若目标文件已存在 → 读取已有配置，与新默认值合并后写回，保留用户自定义值。
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

	// 文件已存在 → 合并模式
	if _, err := os.Stat(cfgPath); err == nil {
		return mergeAndWrite(cfgPath)
	}

	// 文件不存在 → 直接写入模板
	if err := os.WriteFile(cfgPath, embeds.DefaultConfigYAML, 0644); err != nil {
		return fmt.Errorf("write config to %s: %w", cfgPath, err)
	}
	fmt.Printf("config file created: %s\n", cfgPath)
	return nil
}

// mergeAndWrite 读取已有配置文件，与默认值合并后写回
func mergeAndWrite(cfgPath string) error {
	existing, err := Load(cfgPath)
	if err != nil {
		return fmt.Errorf("read existing config: %w", err)
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal merged config: %w", err)
	}

	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return fmt.Errorf("write merged config to %s: %w", cfgPath, err)
	}
	fmt.Printf("config file updated: %s\n", cfgPath)
	return nil
}

// Load 搜索并加载配置文件。
// 返回合并了默认值的完整配置，文件不存在时返回内置默认配置。
func Load(path string) (*Config, error) {
	if path == "" {
		path = AppConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return cfg, nil
}
