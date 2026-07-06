package message

import (
	"context"

	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/logging"
)

// NoopNotifier 不执行任何通知操作
type NoopNotifier struct{}

// Notify 实现了 agentwatcher.Notifier，仅记录日志不发送实际通知
func (n *NoopNotifier) Notify(ctx context.Context, event agentwatcher.AgentEvent) error {
	logging.Debugf("event received but no notifier configured type=%s agent=%s", event.Type, event.Agent.ShortID)
	return nil
}
