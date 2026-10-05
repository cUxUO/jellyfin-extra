//go:build !windows

// Package proc 設定子行程的平台相關屬性。
package proc

import "os/exec"

// NoWindow 只在 Windows 有作用。
func NoWindow(cmd *exec.Cmd) {}
