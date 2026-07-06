package logging

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateLogger 测试创建日志器
func TestCreateLogger(t *testing.T) {
	tmpDir := os.TempDir()
	logFile := filepath.Join(tmpDir, "test_logger.log")
	defer os.Remove(logFile)

	config := NewLogConfig("debug", logFile, "TLCM")
	logger, err := CreateLogger("test", config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	// 测试日志输出
	logger.Debugf("Debug message")
	logger.Infof("Info message")
	logger.Warnf("Warn message")
	logger.Errorf("Error message")

	// 验证日志器已创建
	if logger == nil {
		t.Error("Expected logger to be created")
	}
}

// TestCreateLoggerDuplicate 测试重复创建日志器
func TestCreateLoggerDuplicate(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	_, err := CreateLogger("duplicate", config)
	if err != nil {
		t.Fatalf("Failed to create first logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	// 尝试创建同名日志器
	_, err = CreateLogger("duplicate", config)
	if err == nil {
		t.Error("Expected error when creating duplicate logger")
	}
}

// TestGetLogger 测试获取日志器
func TestGetLogger(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	_, err := CreateLogger("gettest", config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	logger, exists := GetLogger("gettest")
	if !exists {
		t.Error("Expected logger to exist")
	}
	if logger == nil {
		t.Error("Expected logger to be not nil")
	}

	// 测试获取不存在的日志器
	_, exists = GetLogger("nonexistent")
	if exists {
		t.Error("Expected logger to not exist")
	}
}
