package message

import (
	"context"

	"github.com/winezer0/paseo-notifier/agentwatcher"
	"github.com/winezer0/paseo-notifier/logging"
)

// EventFilterNotifier 装饰器：按事件类型开关过滤通知
type EventFilterNotifier struct {
	inner   agentwatcher.Notifier
	enabled map[string]bool
}

// Notify 实现 agentwatcher.Notifier，过滤未启用的事件
func (n *EventFilterNotifier) Notify(ctx context.Context, event agentwatcher.AgentEvent) error {
	if !IsEventEnabled(n.enabled, event.Type) {
		logging.Debugf("notification disabled by config event=%s agent=%s", event.Type, event.Agent.ShortID)
		return nil
	}
	return n.inner.Notify(ctx, event)
}

// IsEventEnabled 检查事件是否启用（未配置默认启用）
func IsEventEnabled(enabled map[string]bool, eventType agentwatcher.EventType) bool {
	on, exists := enabled[string(eventType)]
	if !exists {
		return true
	}
	return on
}

// WrapEventFilter 将 Notifier 包装为带事件过滤的 Notifier
// events 为空时直接返回原始 Notifier，避免无意义的包装层
func WrapEventFilter(inner agentwatcher.Notifier, events map[string]bool) agentwatcher.Notifier {
	if len(events) == 0 {
		return inner
	}
	return &EventFilterNotifier{inner: inner, enabled: events}
}
