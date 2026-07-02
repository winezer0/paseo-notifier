//go:build windows

package message

import "syscall"

// isSystemChinese 通过 Windows API 检测系统界面语言是否为中文
// 中文主语言 ID 为 0x04（zh-CN=0x0804, zh-TW=0x0404, zh-HK=0x0C04 等）
func isSystemChinese() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultUILanguage")
	ret, _, _ := proc.Call()
	langID := uint16(ret)
	primaryLang := langID & 0x3FF
	return primaryLang == 0x04
}
