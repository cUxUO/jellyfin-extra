// Package proc 設定子行程的平台相關屬性。
package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// NoWindow 讓主控台程式（ffmpeg、nvidia-smi）不開自己的主控台視窗。
// xcode 以 GUI 程式建置，沒有主控台可以繼承，不設定的話每啟動一次就閃一個視窗。
func NoWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
