package message

import (
	"errors"
	"fmt"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/lark"
	"gopkg.in/yaml.v3"
)

func init() {
	RegisterProvider("lark_webhook", newLarkWebhookProvider)
}

type larkWebhookConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

func newLarkWebhookProvider(rawCfg yaml.Node) (notify.Notifier, error) {
	var cfg larkWebhookConfig
	if err := rawCfg.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse lark webhook config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return nil, errors.New("lark_webhook: webhook_url is required")
	}

	return lark.NewWebhookService(cfg.WebhookURL), nil
}
