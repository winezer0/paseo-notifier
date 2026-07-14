package message

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/lark"

	larklib "github.com/go-lark/lark"
	"gopkg.in/yaml.v3"
)

func init() {
	RegisterProvider("lark_webhook", newLarkWebhookProvider)
}

// larkWebhookConfig 飞书 Webhook 配置结构体
type larkWebhookConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Secret     string `yaml:"secret"`
}

// newLarkWebhookProvider 根据 YAML 配置创建飞书 Webhook 通知服务。
// 若配置了 secret，则启用 HMAC-SHA256 签名安全验证；
// 若未配置 secret，则回退到无签名模式（向下兼容未启用安全设置的机器人）。
func newLarkWebhookProvider(rawCfg yaml.Node) (notify.Notifier, error) {
	var cfg larkWebhookConfig
	if err := rawCfg.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse lark webhook config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return nil, errors.New("lark_webhook: webhook_url is required")
	}

	// 配置了签名密钥 → 使用带签名验证的自定义服务
	if cfg.Secret != "" {
		return newSignedWebhookService(cfg.WebhookURL, cfg.Secret), nil
	}

	// 无签名密钥 → 回退到 notify 库的默认 webhook 服务（向后兼容）
	return lark.NewWebhookService(cfg.WebhookURL), nil
}

// signedWebhookService 带 HMAC-SHA256 签名的飞书 Webhook 通知服务。
// 实现 notify.Notifier 接口，每次发送消息时自动计算签名并附加到请求体。
type signedWebhookService struct {
	bot    *larklib.Bot
	secret string
}

// newSignedWebhookService 创建一个带签名能力的 Webhook 服务实例
func newSignedWebhookService(webhookURL, secret string) *signedWebhookService {
	return &signedWebhookService{
		bot:    larklib.NewNotificationBot(webhookURL),
		secret: secret,
	}
}

// Send 发送签名通知消息到飞书群聊。
// 使用富文本（Post）格式，标题为 subject，正文为 message。
// 签名通过 go-lark/lark 库的 MsgBuffer.WithSign() 实现，使用当前时间戳和 HMAC-SHA256 算法。
func (s *signedWebhookService) Send(_ context.Context, subject, message string) error {
	now := time.Now()

	// 构建富文本消息内容
	content := larklib.NewPostBuilder().
		Title(subject).
		TextTag(message, 1, false).
		Render()

	// 构建消息缓冲区，附加签名和时间戳
	mb := larklib.NewMsgBuffer(larklib.MsgPost).Post(content)
	mb.WithSign(s.secret, now.Unix())
	msg := mb.Build()

	// 发送签名消息
	resp, err := s.bot.PostNotificationV2(msg)
	if err != nil {
		return fmt.Errorf("post signed webhook message: %w", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf(
			"signed webhook send failed: code=%d msg=%s (see https://open.feishu.cn/document/ukTMukTMukTM/ugjM14COyUjL4ITN for details)",
			resp.Code, resp.Msg,
		)
	}
	return nil
}
