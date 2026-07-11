package message

import (
	"context"

	"github.com/nikoksr/notify"
	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/logging"
)

// NotifyNotifier 将 notify.Service 适配为 agentwatcher.Notifier
type NotifyNotifier struct{}

// Notify 实现了 agentwatcher.Notifier，构建事件消息并通过 notify.Service 发送
func (n *NotifyNotifier) Notify(ctx context.Context, event agentwatcher.AgentEvent) error {
	subject, content := Build(event)

	logging.Infof("sending notification event=%s subject=%s", event.Type, subject)

	return notify.Send(ctx, subject, content)
}
