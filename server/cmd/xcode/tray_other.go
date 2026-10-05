//go:build !windows

package main

import (
	"log"

	"jellyfin-extra/server/internal/logring"
)

// 系統匣只做 Windows 版；其他平台（開發、測試）一律在主控台執行。
const trayAvailable = false

func runWithTray(configPath, logPath string, logger *log.Logger, events *logring.Ring) {}
