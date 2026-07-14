package message

import (
	"strings"
	"testing"

	"github.com/nikoksr/notify"
	"gopkg.in/yaml.v3"
)

func TestRegisterProvider(t *testing.T) {
	RegisterProvider("test_factory", func(raw yaml.Node) (notify.Notifier, error) {
		return nil, nil
	})

	_, ok := GetProvider("test_factory")
	if !ok {
		t.Fatal("expected provider to be registered")
	}
}

func TestGetProviderNotFound(t *testing.T) {
	_, ok := GetProvider("nonexistent")
	if ok {
		t.Fatal("expected provider to not be found")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	RegisterProvider("dup_test_a", func(raw yaml.Node) (notify.Notifier, error) {
		return nil, nil
	})
	RegisterProvider("dup_test_a", func(raw yaml.Node) (notify.Notifier, error) {
		return nil, nil
	})
}

func TestRegisterNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil factory")
		}
	}()
	RegisterProvider("nil_test", nil)
}

func TestRegisteredProviders(t *testing.T) {
	types := RegisteredProviders()
	if len(types) == 0 {
		t.Fatal("expected at least one registered provider (built-in)")
	}
}

func TestRegisteredProvidersContainsBuiltin(t *testing.T) {
	types := RegisteredProviders()
	typeSet := make(map[string]bool)
	for _, tp := range types {
		typeSet[tp] = true
	}
	expected := []string{"dingtalk", "lark_webhook", "lark_app"}
	for _, e := range expected {
		if !typeSet[e] {
			t.Fatalf("expected built-in provider %q to be registered", e)
		}
	}
}

func TestDingTalkFactoryConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr string
	}{
		{
			name:    "missing access_token",
			yamlStr: "secret: mysecret\n",
			wantErr: "access_token is required",
		},
		{
			name:    "missing secret",
			yamlStr: "access_token: mytoken\n",
			wantErr: "secret is required",
		},
		{
			name:    "empty config",
			yamlStr: "",
			wantErr: "access_token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yamlStr), &node); err != nil {
				t.Fatalf("unmarshal yaml: %v", err)
			}
			_, err := newDingTalkProvider(node)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected err containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestLarkWebhookFactoryConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr string
	}{
		{
			name:    "missing webhook_url",
			yamlStr: "",
			wantErr: "webhook_url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yamlStr), &node); err != nil {
				t.Fatalf("unmarshal yaml: %v", err)
			}
			_, err := newLarkWebhookProvider(node)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected err containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestLarkAppFactoryConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr string
	}{
		{
			name:    "missing app_id",
			yamlStr: "app_secret: mysecret\n",
			wantErr: "app_id is required",
		},
		{
			name:    "missing app_secret",
			yamlStr: "app_id: cli_xxx\n",
			wantErr: "app_secret is required",
		},
		{
			name:    "missing receivers",
			yamlStr: "app_id: cli_xxx\napp_secret: mysecret\n",
			wantErr: "at least one receiver is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yamlStr), &node); err != nil {
				t.Fatalf("unmarshal yaml: %v", err)
			}
			_, err := newLarkAppProvider(node)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected err containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestDingTalkFactoryDecodesConfig(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("access_token: mytoken\nsecret: mysecret\n"), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	svc, err := newDingTalkProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestLarkWebhookFactoryDecodesConfig(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("webhook_url: https://open.feishu.cn/open-apis/bot/v2/hook/xxx\n"), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	svc, err := newLarkWebhookProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestLarkWebhookFactoryDecodesConfigWithSecret(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("webhook_url: https://open.feishu.cn/open-apis/bot/v2/hook/xxx\nsecret: mysecret\n"), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	svc, err := newLarkWebhookProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	// 带 secret 时应返回 signedWebhookService 而非 lark.WebhookService
	if _, ok := svc.(*signedWebhookService); !ok {
		t.Fatalf("expected *signedWebhookService when secret is configured, got %T", svc)
	}
}

func TestLarkAppFactoryDecodesConfig(t *testing.T) {
	var node yaml.Node
	yamlStr := "app_id: cli_xxx\napp_secret: mysecret\nreceivers:\n  - type: chat_id\n    value: oc_test\n"
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	svc, err := newLarkAppProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestLarkAppMultipleReceivers(t *testing.T) {
	var node yaml.Node
	yamlStr := "app_id: cli_xxx\napp_secret: mysecret\nreceivers:\n  - type: chat_id\n    value: oc_group\n  - type: open_id\n    value: ou_user\n"
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	svc, err := newLarkAppProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestLarkAppUnknownReceiverType(t *testing.T) {
	var node yaml.Node
	yamlStr := "app_id: cli_xxx\napp_secret: mysecret\nreceivers:\n  - type: invalid_type\n    value: xxx\n"
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	_, err := newLarkAppProvider(node)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown receiver type") {
		t.Fatalf("expected err containing %q, got %q", "unknown receiver type", err.Error())
	}
}

func TestBuildLarkReceiver(t *testing.T) {
	tests := []struct {
		typ   string
		value string
		name  string
	}{
		{"open_id", "ou_xxx", "open_id"},
		{"user_id", "ui_xxx", "user_id"},
		{"union_id", "un_xxx", "union_id"},
		{"email", "test@example.com", "email"},
		{"chat_id", "oc_xxx", "chat_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver, err := buildLarkReceiver(tt.typ, tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if receiver == nil {
				t.Fatal("expected non-nil receiver")
			}
		})
	}
}
