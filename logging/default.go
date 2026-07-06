package logging

import (
	"fmt"
	"sync"
)

var defaultLogger *Logger       // 旧版本默认日志器
var defaultLoggerOnce sync.Once // 用于确保 defaultLogger 只初始化一次

// InitDefaultLogger 旧版本初始化函数，兼容老代码
func InitDefaultLogger(config LogConfig) error {
	var err error
	defaultLogger, err = CreateLogger("default", config)
	return err
}

// NewDefaultLogger 旧版本初始化函数，兼容老代码
func NewDefaultLogger(level, logFile, consoleFormat string) error {
	return InitDefaultLogger(NewLogConfig(level, logFile, consoleFormat))
}

// ensureDefaultLogger 确保 defaultLogger 已初始化，如果未初始化则自动初始化（线程安全）
func ensureDefaultLogger() {
	if defaultLogger == nil {
		defaultLoggerOnce.Do(func() {
			if err := InitDefaultLogger(NewLogConfigEmpty()); err != nil {
				fmt.Printf("init logger error: %v\n", err)
			}
		})
	}
}

// Sync 旧版本全局刷新函数
func Sync() error {
	ensureDefaultLogger()
	if defaultLogger != nil {
		return defaultLogger.Sync()
	}
	return nil
}

// 旧版本全局日志函数，直接转发到 default 日志器
func Debugf(template string, args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Debugf(template, args...)
	}
}

func Infof(template string, args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Infof(template, args...)
	}
}

func Warnf(template string, args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Warnf(template, args...)
	}
}

func Errorf(template string, args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Errorf(template, args...)
	}
}

func Fatalf(template string, args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Fatalf(template, args...)
	}
}

func Debug(args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Debug(args...)
	}
}

func Info(args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Info(args...)
	}
}

func Warn(args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Warn(args...)
	}
}

func Error(args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Error(args...)
	}
}

func Fatal(args ...interface{}) {
	ensureDefaultLogger()
	if defaultLogger != nil {
		defaultLogger.Fatal(args...)
	}
}
