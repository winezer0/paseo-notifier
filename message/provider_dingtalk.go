package message

import (
	"errors"
	"fmt"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/dingding"
	"gopkg.in/yaml.v3"
)

func init() {
	RegisterProvider("dingtalk", newDingTalkProvider)
}

type dingtalkConfig struct {
	AccessToken string `yaml:"access_token"`
	Secret      string `yaml:"secret"`
}

// newDingTalkProvider 根据 YAML 配置创建钉钉通知服务
func newDingTalkProvider(rawCfg yaml.Node) (notify.Notifier, error) {
	var cfg dingtalkConfig
	if err := rawCfg.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse dingtalk config: %w", err)
	}
	if cfg.AccessToken == "" {
		return nil, errors.New("dingtalk: access_token is required")
	}
	if cfg.Secret == "" {
		return nil, errors.New("dingtalk: secret is required")
	}

	return dingding.New(&dingding.Config{
		Token:  cfg.AccessToken,
		Secret: cfg.Secret,
	}), nil
}
