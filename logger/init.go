package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// GlobalLogWriter 全局日志写入器，用于程序退出时关闭日志文件
var GlobalLogWriter io.Closer

// LogFormat 日志格式常量
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

// InitLogger 初始化全局 slog 日志
// logPath: 日志文件路径，空=仅控制台输出
// format: text/json，默认text
// logConsole: 是否开启控制台输出
// level: 日志输出级别 Debug/Info/Warn/Error
func InitLogger(logPath, format string, logConsole bool, level slog.Level) error {
	// 日志级别配置
	opts := &slog.HandlerOptions{Level: level}

	var writers []io.Writer
	// 1. 文件输出
	if logPath != "" {
		rw := newRotatingWriter(logPath)
		if rw == nil {
			return fmt.Errorf("create rotate log writer failed, path=%s", logPath)
		}
		// 保存可关闭句柄，程序退出统一 Close()
		GlobalLogWriter = rw
		writers = append(writers, rw)
	}

	// 2. 控制台输出：显式开启 或 无文件输出时强制控制台兜底
	if logConsole || len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	// MultiWriter 兼容单/多writer，无需分支判断
	multiWriter := io.MultiWriter(writers...)

	// 构建handler
	var handler slog.Handler
	switch format {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(multiWriter, opts)
	default:
		handler = slog.NewTextHandler(multiWriter, opts)
	}
	slog.SetDefault(slog.New(handler))

	// 打印日志输出位置提示
	switch len(writers) {
	case 1:
		if logPath == "" {
			slog.Info("logger init success", "output", "stdout only")
		} else {
			slog.Info("logger init success", "output", "file only", "path", logPath)
		}
	case 2:
		slog.Info("logger init success", "output", "file+stdout", "path", logPath)
	}

	return nil
}

// CloseLogger 关闭日志文件句柄，程序退出调用
func CloseLogger() {
	if GlobalLogWriter != nil {
		_ = GlobalLogWriter.Close()
	}
}
