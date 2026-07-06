package message

import (
	"context"
	"fmt"
	"strings"

	"github.com/nikoksr/notify"
	"gopkg.in/yaml.v3"
)

// consoleConfig console 供应商的配置
type consoleConfig struct {
	Enable bool `yaml:"enable"`
}

// emojiReplacer Slack 风格 emoji 代码到 Unicode 字符的映射
var emojiReplacer = strings.NewReplacer(
	":white_check_mark:", "✅",
	":x:", "❌",
	":warning:", "⚠️",
	":bell:", "🔔",
)

// replaceEmoji 将文本中的 Slack 风格 emoji 代码替换为 Unicode 字符
func replaceEmoji(s string) string {
	return emojiReplacer.Replace(s)
}

// consoleService 实现 notify.Notifier，在控制台输出带 emoji 的通知
type consoleService struct{}

// Send 实现 notify.Notifier 接口，先替换 emoji 再输出到控制台
func (s *consoleService) Send(ctx context.Context, subject, message string) error {
	subject = replaceEmoji(subject)
	message = replaceEmoji(message)

	fmt.Println()
	fmt.Println("=== Notification ===")
	fmt.Println("Subject:", subject)
	for _, line := range strings.Split(message, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println("====================")
	fmt.Println()
	return nil
}

func init() {
	RegisterProvider("console", newConsoleProvider)
}

func newConsoleProvider(rawCfg yaml.Node) (notify.Notifier, error) {
	var cfg consoleConfig
	if err := rawCfg.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("console: parse config failed: %w", err)
	}
	if !cfg.Enable {
		return nil, fmt.Errorf("console: disabled")
	}
	return &consoleService{}, nil
}