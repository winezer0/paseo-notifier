package logging

import "testing"

// TestNewLogConfig 测试日志配置创建
func TestNewLogConfig(t *testing.T) {
	config := NewLogConfig("debug", "test.log", "TLCM")
	if config.Level != "debug" {
		t.Errorf("Expected level 'debug', got '%s'", config.Level)
	}
	if config.LogFile != "test.log" {
		t.Errorf("Expected log file 'test.log', got '%s'", config.LogFile)
	}
	if config.ConsoleFormat != "TLCM" {
		t.Errorf("Expected console format 'TLCM', got '%s'", config.ConsoleFormat)
	}
	if config.MaxSize != 100 {
		t.Errorf("Expected max size 100, got %d", config.MaxSize)
	}
	if config.MaxBackups != 10 {
		t.Errorf("Expected max backups 10, got %d", config.MaxBackups)
	}
	if config.MaxAge != 30 {
		t.Errorf("Expected max age 30, got %d", config.MaxAge)
	}
	if !config.Compress {
		t.Error("Expected compress to be true")
	}
}

// TestNewLogConfigEmpty 测试空配置创建
func TestNewLogConfigEmpty(t *testing.T) {
	config := NewLogConfigEmpty()
	if config.Level != "info" {
		t.Errorf("Expected level 'info', got '%s'", config.Level)
	}
	if config.LogFile != "" {
		t.Errorf("Expected empty log file, got '%s'", config.LogFile)
	}
	if config.ConsoleFormat != "LCM" {
		t.Errorf("Expected console format 'LCM', got '%s'", config.ConsoleFormat)
	}
	if config.MaxSize != 100 {
		t.Errorf("Expected max size 100, got %d", config.MaxSize)
	}
	if config.MaxBackups != 3 {
		t.Errorf("Expected max backups 3, got %d", config.MaxBackups)
	}
	if config.MaxAge != 30 {
		t.Errorf("Expected max age 30, got %d", config.MaxAge)
	}
	if !config.Compress {
		t.Error("Expected compress to be true")
	}
}
