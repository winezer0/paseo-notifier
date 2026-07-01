package message

import (
	"fmt"
	"sync"

	"github.com/nikoksr/notify"
	"gopkg.in/yaml.v3"
)

// ProviderFactory 从配置创建 notify.Notifier 的工厂函数
// rawConfig 对应配置文件中 providers[].config 节点的 YAML 内容
type ProviderFactory func(rawConfig yaml.Node) (notify.Notifier, error)

var (
	providerMu     sync.RWMutex
	providerGblReg = make(map[string]ProviderFactory)
)

// RegisterProvider 注册一个通知供应商工厂
// typ 为供应商类型名称（如 dingtalk、lark_webhook），对应配置中的 type 字段
func RegisterProvider(typ string, factory ProviderFactory) {
	providerMu.Lock()
	defer providerMu.Unlock()
	if factory == nil {
		panic(fmt.Sprintf("provider %q: factory must not be nil", typ))
	}
	if _, dup := providerGblReg[typ]; dup {
		panic(fmt.Sprintf("provider %q already registered", typ))
	}
	providerGblReg[typ] = factory
}

// GetProvider 根据类型名查找已注册的供应商工厂
func GetProvider(typ string) (ProviderFactory, bool) {
	providerMu.RLock()
	defer providerMu.RUnlock()
	f, ok := providerGblReg[typ]
	return f, ok
}

// RegisteredProviders 返回所有已注册的供应商类型列表
func RegisteredProviders() []string {
	providerMu.RLock()
	defer providerMu.RUnlock()
	types := make([]string, 0, len(providerGblReg))
	for t := range providerGblReg {
		types = append(types, t)
	}
	return types
}
