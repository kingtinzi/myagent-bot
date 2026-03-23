//go:build windows

package main

import (
	"runtime"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	systraySetIcon    = systray.SetIcon
	systraySetTooltip = systray.SetTooltip
)

// startSystray 在 Windows 上通过系统托盘提供菜单。
// systray 必须在锁定的 OS 线程中运行，否则首次点击菜单后左/右键会失效（见 getlantern/systray#269）
func startSystray(a *App) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		systray.Run(func() { runTray(a) }, func() {})
	}()
}

func runTray(a *App) {
	configureTrayAppearance()
	mShow := systray.AddMenuItem("显示主窗口", "显示主窗口")
	mSettings := systray.AddMenuItem("设置", "打开设置")
	mQuit := systray.AddMenuItem("退出", "退出程序")
	go func() {
		for range mShow.ClickedCh {
			wailsruntime.WindowShow(a.ctx)
		}
	}()
	go func() {
		for range mSettings.ClickedCh {
			a.OpenSettings()
		}
	}()
	go func() {
		for range mQuit.ClickedCh {
			wailsruntime.Quit(a.ctx)
		}
	}()
}

func configureTrayAppearance() {
	if len(trayIcon) > 0 {
		systraySetIcon(trayIcon)
	}
	systraySetTooltip("PinchBot")
}

func quitSystray() {
	systray.Quit()
}
