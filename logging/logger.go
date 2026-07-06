package logging

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// -------------------------- 日志器实现 --------------------------

// Logger 日志器实例，线程安全
type Logger struct {
	zapLogger *zap.Logger
	sugar     *zap.SugaredLogger
	config    LogConfig
	mu        sync.RWMutex
}

// init 初始化日志器核心(已修正EncodeTime配置)
func (l *Logger) init() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 解析日志级别
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(l.config.Level)); err != nil {
		level = zapcore.InfoLevel // 默认为info级别
	}

	// 准备输出核心
	var cores []zapcore.Core

	// 控制台输出
	if l.config.ConsoleFormat != "" && l.config.ConsoleFormat != "off" {
		encoder := newConsoleEncoder(l.config.ConsoleFormat)
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.Lock(os.Stdout),
			level,
		))
	}

	// 文件输出(带日志轮转，正确配置时间格式)
	if l.config.LogFile != "" {
		if err := ensureDir(l.config.LogFile); err != nil {
			return fmt.Errorf("failed to create log dir: %w", err)
		}

		// 日志轮转配置
		rotator := &lumberjack.Logger{
			Filename:   l.config.LogFile,
			MaxSize:    l.config.MaxSize,    // 单个文件最大100MB
			MaxBackups: l.config.MaxBackups, // 最多保留10个备份
			MaxAge:     l.config.MaxAge,     // 保留30天
			Compress:   l.config.Compress,   // 压缩备份文件
		}

		// 配置文件日志编码器(含时间格式)
		fileEncoderCfg := zap.NewProductionEncoderConfig()
		// 配置时间格式为ISO8601(如：2024-05-20T15:30:00.000Z)
		fileEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		// (可选)自定义时间格式示例(如：2024-05-20 15:30:00.000)
		// fileEncoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		// 	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
		// }

		// 创建JSON格式编码器
		fileEncoder := zapcore.NewJSONEncoder(fileEncoderCfg)
		cores = append(cores, zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(rotator),
			level,
		))
	}

	if len(cores) == 0 {
		return fmt.Errorf("no log output (console/file) has been configured")
	}

	// 创建zap日志器
	l.zapLogger = zap.New(
		zapcore.NewTee(cores...),
		zap.AddCaller(),      // 显示调用位置(如 main.go:20)
		zap.AddCallerSkip(2), // 跳过内部方法，显示真实业务代码位置
	)
	l.sugar = l.zapLogger.Sugar()

	return nil
}

// 日志输出方法
func (l *Logger) Debugf(template string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Debugf(template, args...)
	}
}

func (l *Logger) Infof(template string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Infof(template, args...)
	}
}

func (l *Logger) Warnf(template string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Warnf(template, args...)
	}
}

func (l *Logger) Errorf(template string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Errorf(template, args...)
	}
}

func (l *Logger) Fatalf(template string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Fatalf(template, args...)
	}
}

func (l *Logger) Debug(args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Debug(args...)
	}
}

func (l *Logger) Info(args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Info(args...)
	}
}

func (l *Logger) Warn(args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Warn(args...)
	}
}

func (l *Logger) Error(args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Error(args...)
	}
}

func (l *Logger) Fatal(args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.sugar != nil {
		l.sugar.Fatal(args...)
	}
}

// Sync 刷新日志缓冲区
func (l *Logger) Sync() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.zapLogger != nil {
		return l.zapLogger.Sync()
	}
	return nil
}
