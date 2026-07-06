package logging

import (
	"go.uber.org/zap/zapcore"
	"os"
	"path/filepath"
	"strings"
)

// -------------------------- 工具函数 --------------------------

// 创建控制台编码器
func newConsoleEncoder(format string) zapcore.Encoder {
	cfg := zapcore.EncoderConfig{
		TimeKey:      "T",
		LevelKey:     "L",
		CallerKey:    "C",
		MessageKey:   "M",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}

	if !strings.Contains(format, "T") {
		cfg.TimeKey = ""
	}
	if !strings.Contains(format, "L") {
		cfg.LevelKey = ""
	}
	if !strings.Contains(format, "C") {
		cfg.CallerKey = ""
	}
	if !strings.Contains(format, "M") {
		cfg.MessageKey = ""
	}

	return zapcore.NewConsoleEncoder(cfg)
}

// ensureDir 确保目录存在
func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
