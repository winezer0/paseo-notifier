package logging

import (
	"errors"
	"fmt"
	"sync"
)

// -------------------------- 日志器管理器 --------------------------

type loggerManager struct {
	loggers map[string]*Logger
	mu      sync.RWMutex
}

// GetLogger 获取已创建的日志器
func (manager *loggerManager) GetLogger(name string) (*Logger, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	logger, exists := manager.loggers[name]
	return logger, exists
}

// CloseAll 关闭所有日志器
func (manager *loggerManager) CloseAll() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	var errList []error
	for name, logger := range manager.loggers {
		if err := logger.Sync(); err != nil {
			errList = append(errList, fmt.Errorf("close log recorder '%s' error: %w", name, err))
		}
	}
	manager.loggers = make(map[string]*Logger)
	return errors.Join(errList...)
}

var (
	globalManager *loggerManager
	once          sync.Once
)

func createManagerOnce() *loggerManager {
	once.Do(func() {
		globalManager = &loggerManager{
			loggers: make(map[string]*Logger),
		}
	})
	return globalManager
}

// CreateLogger 创建新的日志器
func (manager *loggerManager) CreateLogger(name string, config LogConfig) (*Logger, error) {
	if name == "" {
		return nil, fmt.Errorf("log recorder name cannot be empty")
	}
	manager.mu.RLock()
	_, exists := manager.loggers[name]
	manager.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("log recorder already exist: %s", name)
	}

	logger := &Logger{config: config}
	if err := logger.init(); err != nil {
		return nil, err
	}

	manager.mu.Lock()
	manager.loggers[name] = logger
	manager.mu.Unlock()

	return logger, nil
}

// CreateLogger 创建新的日志器
func CreateLogger(name string, config LogConfig) (*Logger, error) {
	manager := createManagerOnce()
	return manager.CreateLogger(name, config)
}

// GetLogger 获取已创建的日志器
func GetLogger(name string) (*Logger, bool) {
	manager := createManagerOnce()
	return manager.GetLogger(name)
}

// CloseAll 关闭所有日志器
func CloseAll() error {
	manager := createManagerOnce()
	return manager.CloseAll()
}
