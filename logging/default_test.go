package logging

import "testing"

// TestInitDefaultLogger 测试初始化默认日志器
func TestInitDefaultLogger(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	err := InitDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	// 验证默认日志器已创建
	if defaultLogger == nil {
		t.Error("Expected defaultLogger to be initialized")
	}
}

// TestSync 测试同步日志
func TestSync(t *testing.T) {
	config := NewLogConfig("debug", "", "TLCM")
	err := InitDefaultLogger(config)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		_ = CloseAll()
	}()

	err = Sync()
	if err != nil {
		t.Errorf("Failed to sync logger: %v", err)
	}
}

// TestEnsureDefaultLogger 测试自动初始化默认日志器
func TestEnsureDefaultLogger(t *testing.T) {
	// 注意：由于使用了 sync.Once，无法在测试中重置 defaultLogger
	// 这里只验证全局日志函数能正常工作

	// 调用全局日志函数
	Info("Test auto-init message")

	// 验证 defaultLogger 已被初始化
	if defaultLogger == nil {
		t.Error("Expected defaultLogger to be auto-initialized")
	}

	defer func() {
		_ = CloseAll()
	}()
}

// TestGlobalLogFunctions 测试全局日志函数
func TestGlobalLogFunctions(t *testing.T) {
	defer func() {
		_ = CloseAll()
	}()

	// 测试在未初始化情况下自动初始化
	Infof("Auto-init info message")
	Warnf("Auto-init warn message")
	Errorf("Auto-init error message")
	Debugf("Auto-init debug message")

	Info("Auto-init info")
	Warn("Auto-init warn")
	Error("Auto-init error")
	Debug("Auto-init debug")

	// 验证默认日志器已创建
	if defaultLogger == nil {
		t.Error("Expected defaultLogger to be auto-initialized")
	}
}
