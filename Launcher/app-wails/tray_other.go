//go:build !windows

package main

// macOS：Wails 与 getlantern/systray 均定义 ObjC AppDelegate，同进程链接会 duplicate symbol。
// Linux：当前与 Windows 行为对齐为无独立托盘线程（可用 Dock / 窗口）。
// 托盘菜单仅在 Windows 构建中启用；测试通过可注入的 systraySetIcon / systraySetTooltip 验证 configureTrayAppearance。

var (
	systraySetIcon    = func(icon []byte) { _ = icon }
	systraySetTooltip = func(tooltip string) { _ = tooltip }
)

func startSystray(_ *App) {
	// 无系统托盘：用户通过 Dock 图标或主窗口使用应用。
}

func configureTrayAppearance() {
	if len(trayIcon) > 0 {
		systraySetIcon(trayIcon)
	}
	systraySetTooltip("PinchBot")
}

func quitSystray() {}
