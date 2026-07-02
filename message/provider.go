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

// RegisterProvider 注册指定类型的通知供应商工厂
// 如果 factory 为 nil 或类型已注册则会 panic
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

// GetProvider 根据类型标识查找已注册的供应商工厂
func GetProvider(typ string) (ProviderFactory, bool) {
	providerMu.RLock()
	defer providerMu.RUnlock()
	f, ok := providerGblReg[typ]
	return f, ok
}

// RegisteredProviders 返回所有已注册供应商类型标识的列表
func RegisteredProviders() []string {
	providerMu.RLock()
	defer providerMu.RUnlock()
	types := make([]string, 0, len(providerGblReg))
	for t := range providerGblReg {
		types = append(types, t)
	}
	return types
}
