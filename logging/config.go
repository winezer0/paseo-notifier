package logging

// LogConfig 日志配置结构体
type LogConfig struct {
	Level         string // 日志级别: debug/info/warn/error/fatal
	LogFile       string // 日志文件路径，空串表示不输出到文件
	ConsoleFormat string // 控制台格式: 空串或"off"表示关闭，支持"T(时间)L(级别)C(调用者)M(消息)"
	MaxSize       int    // 单个日志文件文件最大大小
	MaxBackups    int    // 最多保留几个日志文件备份
	MaxAge        int    // 日志文件保留多少天
	Compress      bool   // 日志备份文件是否压缩
}

// NewLogConfig 创建日志配置实例，提供默认值
func NewLogConfig(level, logFile, consoleFormat string) LogConfig {
	if level == "" {
		level = "info" // 默认info级别
	}
	if consoleFormat == "" {
		consoleFormat = "LCM"
	}
	return LogConfig{
		Level:         level,
		LogFile:       logFile,
		ConsoleFormat: consoleFormat,
		MaxSize:       100,  // 单个文件最大100MB
		MaxBackups:    10,   // 最多保留10个备份
		MaxAge:        30,   // 保留30天
		Compress:      true, // 压缩备份文件
	}
}

// NewLogConfigEmpty 创建日志配置实例，提供全部默认值
func NewLogConfigEmpty() LogConfig {
	return LogConfig{
		Level:         "info",
		LogFile:       "",
		ConsoleFormat: "LCM",
		MaxSize:       100,  // 单个文件最大100MB
		MaxBackups:    3,    // 最多保留10个备份
		MaxAge:        30,   // 保留30天
		Compress:      true, // 压缩备份文件
	}
}
