//go:build !windows

package message

// isSystemChinese 非 Windows 平台的占位实现
// 语言检测通过 detectLang 中的 LANG/LC_ALL/LC_MESSAGES 环境变量完成
func isSystemChinese() bool {
	return false
}
