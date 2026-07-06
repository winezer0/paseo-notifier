package logging

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCloseAll 测试关闭所有日志器
func TestCloseAll(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	_, err := CreateLogger("close1", config)
	if err != nil {
		t.Fatalf("Failed to create logger 1: %v", err)
	}

	_, err = CreateLogger("close2", config)
	if err != nil {
		t.Fatalf("Failed to create logger 2: %v", err)
	}

	err = CloseAll()
	if err != nil {
		t.Errorf("Failed to close all loggers: %v", err)
	}

	// 验证日志器已被清空
	_, exists := GetLogger("close1")
	if exists {
		t.Error("Expected logger to be closed")
	}
}

// TestLoggerWithFileOutput 测试带文件输出的日志器
func TestLoggerWithFileOutput(t *testing.T) {
	tmpDir := os.TempDir()
	logFile := filepath.Join(tmpDir, "test_file_output.log")
	defer os.Remove(logFile)

	config := NewLogConfig("debug", logFile, "off")
	logger, err := CreateLogger("filetest", config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	logger.Infof("Test file output message")
	_ = logger.Sync()

	// 验证日志文件已创建
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Expected log file to be created")
	}
}

// TestLoggerConcurrent 测试日志器并发安全性
func TestLoggerConcurrent(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	logger, err := CreateLogger("concurrent", config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			logger.Infof("Concurrent message %d", id)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
