//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"jellyfin-extra/server/internal/logring"
)

const trayAvailable = true

// icon.ico 由 tools/make_xcode_icon.py 產生；exe 本身的圖示資源（rsrc_windows_amd64.syso）也是從它做的。
//
//go:embed icon.ico
var trayIcon []byte

const (
	appTitle = "Jellyfin Extra 轉碼伺服器"
	// 登入時自動啟動：寫在目前使用者的 Run 機碼，不需要系統管理員權限
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue   = "JellyfinExtraXcode"
)

// runWithTray 在系統匣常駐執行伺服器，直到從選單結束或伺服器無法執行。
func runWithTray(configPath, logPath string, logger *log.Logger, events *logring.Ring) {
	// 同一個登入工作階段只允許一份：再次雙擊時提示，而不是因為連接埠被占用而報錯
	name, _ := windows.UTF16PtrFromString(`Local\JellyfinExtraXcode`)
	mutex, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		messageBox("轉碼伺服器已經在執行中，請在系統匣找琥珀色的播放圖示。", windows.MB_ICONINFORMATION)
		return
	}
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	systray.Run(func() {
		onTrayReady(ctx, cancel, done, configPath, logPath, logger, events)
	}, func() {
		cancel()
		<-done // 等所有 ffmpeg 停掉、暫存清掉
	})
}

func onTrayReady(ctx context.Context, cancel context.CancelFunc, done chan struct{},
	configPath, logPath string, logger *log.Logger, events *logring.Ring) {
	systray.SetIcon(trayIcon)
	systray.SetTitle(appTitle)
	systray.SetTooltip(appTitle + "：啟動中")

	mStatus := systray.AddMenuItem("啟動中…", "")
	mStatus.Disable()
	systray.AddSeparator()
	mDash := systray.AddMenuItem("開啟監控頁", "在瀏覽器開啟監控頁")
	mDash.Disable() // 開始接受連線後才能用
	mLog := systray.AddMenuItem("開啟記錄檔", logPath)
	mDir := systray.AddMenuItem("開啟所在資料夾", "設定檔 xcode.json、記錄檔與 ffmpeg 所在的資料夾")
	mAuto := systray.AddMenuItemCheckbox("登入時自動啟動", "登入 Windows 後自動在系統匣啟動", autostartEnabled(configPath))
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("結束", "停止所有轉碼並結束")

	ready := make(chan status, 1)
	go func() {
		err := run(ctx, configPath, logger, events, func(st status) { ready <- st })
		if err != nil {
			logger.Print(err)
			messageBox(fmt.Sprintf("轉碼伺服器無法執行：\n\n%v\n\n記錄檔：%s", err, logPath), windows.MB_ICONERROR)
		}
		close(done)
		systray.Quit()
	}()

	go func() {
		var st status
		update := func() {
			text := "待命中"
			if n := st.Active(); n > 0 {
				text = fmt.Sprintf("轉碼中：%d 個", n)
			}
			mStatus.SetTitle(text)
			systray.SetTooltip(appTitle + "：" + text)
		}
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			select {
			case st = <-ready:
				mDash.Enable()
				update()
			case <-tick.C:
				if st.Active != nil && ctx.Err() == nil {
					update()
				}
			case <-mDash.ClickedCh:
				shellOpen(st.DashboardURL)
			case <-mLog.ClickedCh:
				shellOpen(logPath)
			case <-mDir.ClickedCh:
				shellOpen(filepath.Dir(logPath))
			case <-mAuto.ClickedCh:
				on := !mAuto.Checked()
				if err := setAutostart(on, configPath); err != nil {
					logger.Printf("autostart: %v", err)
					messageBox("無法變更自動啟動設定："+err.Error(), windows.MB_ICONERROR)
					break
				}
				if on {
					mAuto.Check()
				} else {
					mAuto.Uncheck()
				}
				logger.Printf("autostart at login: %v", on)
			case <-mQuit.ClickedCh:
				mStatus.SetTitle("正在停止…")
				systray.SetTooltip(appTitle + "：正在停止")
				logger.Print("quit from tray")
				cancel()
			case <-done:
				return
			}
		}
	}()
}

// autostartCommand 是寫進 Run 機碼的命令；設定檔不在預設位置時一併帶上。
func autostartCommand(configPath string) string {
	exe, _ := os.Executable()
	cmd := `"` + exe + `"`
	if !strings.EqualFold(configPath, filepath.Join(filepath.Dir(exe), "xcode.json")) {
		cmd += ` -config "` + configPath + `"`
	}
	return cmd
}

func autostartEnabled(configPath string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValue)
	return err == nil && strings.EqualFold(v, autostartCommand(configPath))
}

func setAutostart(on bool, configPath string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if on {
		return k.SetStringValue(runValue, autostartCommand(configPath))
	}
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// shellOpen 用預設程式開啟網址、檔案或資料夾。
func shellOpen(target string) {
	if target == "" {
		return
	}
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(target)
	windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

func messageBox(text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(appTitle)
	windows.MessageBox(0, t, c, windows.MB_OK|icon)
}
