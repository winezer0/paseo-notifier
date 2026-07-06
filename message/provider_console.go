package message

import (
	"context"
	"fmt"
	"strings"

	"github.com/winezer0/paseo-notifier/agentwatcher"
)

// emojiMap Slack 风格 emoji 代码到 Unicode 字符的映射
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

// consoleNotifier 在控制台输出带 emoji 的通知
type consoleNotifier struct{}

// Notify implements agentwatcher.Notifier
func (n *consoleNotifier) Notify(ctx context.Context, event agentwatcher.AgentEvent) error {
	subject, content := Build(event)

	subject = replaceEmoji(subject)
	content = replaceEmoji(content)

	fmt.Println()
	fmt.Println("=== Notification ===")
	fmt.Println("Subject:", subject)
	for _, line := range strings.Split(content, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println("====================")
	fmt.Println()
	return nil
}