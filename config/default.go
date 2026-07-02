package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/winezer0/paseo-notifier/embeds"
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
