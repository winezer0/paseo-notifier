package message

import (
	"errors"
	"fmt"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/lark"
	"gopkg.in/yaml.v3"
)

func init() {
	RegisterProvider("lark_app", newLarkAppProvider)
}

// larkReceiverItem 飞书接收者配置项
type larkReceiverItem struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

type larkAppConfig struct {
	AppID     string             `yaml:"app_id"`
	AppSecret string             `yaml:"app_secret"`
	Receivers []larkReceiverItem `yaml:"receivers"`
}

func newLarkAppProvider(rawCfg yaml.Node) (notify.Notifier, error) {
	var cfg larkAppConfig
	if err := rawCfg.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse lark app config: %w", err)
	}
	if cfg.AppID == "" {
		return nil, errors.New("lark_app: app_id is required")
	}
	if cfg.AppSecret == "" {
		return nil, errors.New("lark_app: app_secret is required")
	}
	if len(cfg.Receivers) == 0 {
		return nil, errors.New("lark_app: at least one receiver is required")
	}

	svc := lark.NewCustomAppService(cfg.AppID, cfg.AppSecret)

	for _, r := range cfg.Receivers {
		receiver, err := buildLarkReceiver(r.Type, r.Value)
		if err != nil {
			return nil, fmt.Errorf("lark_app: %w", err)
		}
		svc.AddReceivers(receiver)
	}

	return svc, nil
}

func buildLarkReceiver(typ, value string) (*lark.ReceiverID, error) {
	switch typ {
	case "open_id":
		return lark.OpenID(value), nil
	case "user_id":
		return lark.UserID(value), nil
	case "union_id":
		return lark.UnionID(value), nil
	case "email":
		return lark.Email(value), nil
	case "chat_id":
		return lark.ChatID(value), nil
	default:
		return nil, fmt.Errorf("unknown receiver type %q, valid types: open_id, user_id, union_id, email, chat_id", typ)
	}
}
