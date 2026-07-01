//go:build !windows

package message

// isSystemChinese 非 Windows 平台的语言检测
// Linux/macOS 的语言检测已通过 LANG/LC_ALL/LC_MESSAGES 环境变量完成
func isSystemChinese() bool {
	return false
}
