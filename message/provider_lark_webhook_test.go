package message

import (
	"context"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	larklib "github.com/go-lark/lark"
)

// 飞书 Webhook 签名测试用的硬编码凭证
const (
	testWebhookURL    = "https://open.feishu.cn/open-apis/bot/v2/hook/6c3bf07d-7760-4443-a678-223c9a3d9757"
	testWebhookSecret = "51atj4LO0mDXfPpDTbiSXe"
)

// TestGenSign 验证签名算法正确性
func TestGenSign(t *testing.T) {
	// 官方文档示例：secret="xxx", timestamp=1661860880 → "QnWVTSBe6FmQDE0bG6X0mURbI+DnvVyu1h+j5dHOjrU="
	sign, err := larklib.GenSign("xxx", 1661860880)
	if err != nil {
		t.Fatalf("GenSign failed: %v", err)
	}
	expected := "QnWVTSBe6FmQDE0bG6X0mURbI+DnvVyu1h+j5dHOjrU="
	if sign != expected {
		t.Fatalf("expected sign %q, got %q", expected, sign)
	}
}

// TestLarkWebhookRealSend 真实发送测试（需要网络连接和有效凭证）
// 运行方式: go test -run TestLarkWebhookRealSend -count=1 -v
func TestLarkWebhookRealSend(t *testing.T) {
	t.Run("send_text_with_signature", func(t *testing.T) {
		bot := larklib.NewNotificationBot(testWebhookURL)

		mb := larklib.NewMsgBuffer(larklib.MsgText)
		mb.Text("[paseo-notifier test] webhook 签名测试消息")
		mb.WithSign(testWebhookSecret, time.Now().Unix())
		msg := mb.Build()

		resp, err := bot.PostNotificationV2(msg)
		if err != nil {
			t.Fatalf("PostNotificationV2 failed: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("send failed: code=%d msg=%s", resp.Code, resp.Msg)
		}
		t.Logf("text message sent successfully: code=%d status=%s", resp.Code, resp.StatusMessage)
	})

	t.Run("send_post_with_signature", func(t *testing.T) {
		bot := larklib.NewNotificationBot(testWebhookURL)

		content := larklib.NewPostBuilder().
			Title("[paseo-notifier test] 富文本通知").
			TextTag("这是一条来自 paseo-notifier 的 webhook 签名测试消息", 1, false).
			TextTag("如果收到此消息，说明 webhook + secret 签名配置正确", 1, false).
			Render()

		mb := larklib.NewMsgBuffer(larklib.MsgPost).Post(content)
		mb.WithSign(testWebhookSecret, time.Now().Unix())
		msg := mb.Build()

		resp, err := bot.PostNotificationV2(msg)
		if err != nil {
			t.Fatalf("PostNotificationV2 failed: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("send failed: code=%d msg=%s", resp.Code, resp.Msg)
		}
		t.Logf("post message sent successfully: code=%d status=%s", resp.Code, resp.StatusMessage)
	})
}

// TestLarkWebhookRealSendWithoutSign 无签名发送测试（如果机器人未开启签名验证则应成功）
func TestLarkWebhookRealSendWithoutSign(t *testing.T) {
	bot := larklib.NewNotificationBot(testWebhookURL)

	mb := larklib.NewMsgBuffer(larklib.MsgText)
	mb.Text("[paseo-notifier test] 无签名测试消息")
	msg := mb.Build()

	resp, err := bot.PostNotificationV2(msg)
	if err != nil {
		t.Fatalf("PostNotificationV2 failed: %v", err)
	}
	t.Logf("unsigned message result: code=%d msg=%s status=%s", resp.Code, resp.Msg, resp.StatusMessage)
	// 注意：如果机器人开启了签名验证，Code 可能非 0
}

// TestLarkWebhookConfigWithSecret 测试带 secret 的 YAML 配置解析
func TestLarkWebhookConfigWithSecret(t *testing.T) {
	yamlStr := "webhook_url: https://open.feishu.cn/open-apis/bot/v2/hook/xxx\nsecret: mysecret123\n"

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	svc, err := newLarkWebhookProvider(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	// 验证 service 实现了 notify.Notifier 接口
	ctx := context.Background()
	err = svc.Send(ctx, "test subject", "test message")
	// 这里预期会失败（假的 webhook URL），但不应 panic
	t.Logf("Send result: %v (expected to fail with fake URL)", err)
}

// TestLarkWebhookConfigWithoutSecret 测试不带 secret 的 YAML 配置解析（向后兼容）
func TestLarkWebhookConfigWithoutSecret(t *testing.T) {
	yamlStr := "webhook_url: https://open.feishu.cn/open-apis/bot/v2/hook/xxx\n"

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
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

// TestLarkWebhookConfigMissingURL 测试缺少 webhook_url 时的错误处理
func TestLarkWebhookConfigMissingURL(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr string
	}{
		{
			name:    "empty config",
			yamlStr: "",
			wantErr: "webhook_url is required",
		},
		{
			name:    "only secret no url",
			yamlStr: "secret: mysecret\n",
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
