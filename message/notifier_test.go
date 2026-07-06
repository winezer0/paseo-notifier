package message

import (
	"testing"

	"github.com/winezer0/paseo-notifier/config"
	"gopkg.in/yaml.v3"
)

// parseProviderItem 从 YAML 字符串解析 ProviderItem（模拟配置文件加载过程）
func parseProviderItem(t *testing.T, yamlStr string) config.ProviderItem {
	t.Helper()
	var item config.ProviderItem
	if err := yaml.Unmarshal([]byte(yamlStr), &item); err != nil {
		t.Fatalf("unmarshal provider item: %v", err)
	}
	return item
}

func TestBuildNotifierEmptyProviders(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = nil

	n := BuildNotifier(cfg)
	if _, ok := n.(*consoleNotifier); !ok {
		t.Fatal("expected consoleNotifier for empty providers (default)")
	}
}

func TestBuildNotifierUnknownProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		{Type: "unknown_xyz"},
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*consoleNotifier); !ok {
		t.Fatal("expected consoleNotifier when all providers are invalid")
	}
}

func TestBuildNotifierWithDingTalk(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		parseProviderItem(t, "type: dingtalk\nconfig:\n  access_token: mytoken\n  secret: mysecret\n"),
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*NotifyNotifier); !ok {
		t.Fatal("expected NotifyNotifier for valid dingtalk config")
	}
}

func TestBuildNotifierMultiProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		parseProviderItem(t, "type: dingtalk\nconfig:\n  access_token: mytoken\n  secret: mysecret\n"),
		parseProviderItem(t, "type: lark_webhook\nconfig:\n  webhook_url: https://open.feishu.cn/open-apis/bot/v2/hook/xxx\n"),
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*NotifyNotifier); !ok {
		t.Fatal("expected NotifyNotifier for multi-provider config")
	}
}

func TestBuildNotifierSkipsInvalidProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		parseProviderItem(t, "type: dingtalk\nconfig:\n  access_token: mytoken\n"),
		parseProviderItem(t, "type: lark_webhook\nconfig:\n  webhook_url: https://example.com/hook\n"),
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*NotifyNotifier); !ok {
		t.Fatal("expected NotifyNotifier when at least one provider is valid")
	}
}

func TestSendStartupNotificationWithNoopNotifier(t *testing.T) {
	n := &noopNotifier{}
	SendStartupNotification(n)
}

func TestBuildNotifierConsoleOnly(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		{Type: "console"},
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*consoleNotifier); !ok {
		t.Fatal("expected consoleNotifier for console-only config")
	}
}

func TestBuildNotifierConsoleWithOtherProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		{Type: "console"},
		parseProviderItem(t, "type: dingtalk\nconfig:\n  access_token: mytoken\n  secret: mysecret\n"),
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*NotifyNotifier); !ok {
		t.Fatal("expected NotifyNotifier when console is combined with other providers")
	}
}

func TestBuildNotifierConsoleWithInvalidOtherProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifier.Providers = []config.ProviderItem{
		{Type: "console"},
		{Type: "unknown_xyz"},
	}

	n := BuildNotifier(cfg)
	if _, ok := n.(*consoleNotifier); !ok {
		t.Fatal("expected consoleNotifier when only console is valid")
	}
}
