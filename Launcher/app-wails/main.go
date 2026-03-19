// Launcher 聊天小窗 — Wails 版：右侧托盘入口 + 内嵌网页
// 前端在 frontend/ 下，可替换为任意开源聊天 UI 再改

package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var frontendAssets embed.FS

const (
	windowWidth  = 380
	windowHeight = 600
	settingsURL  = "http://localhost:18800"
	gatewayURL   = "http://127.0.0.1:18790"
	// macOS 下空标题可能导致窗口/菜单栏激活异常，勿留空。
	title = "PinchBot"
)

// PlatformAPIBaseURL 支持构建时通过 -ldflags 注入固定平台后端地址。
// 例如：-X main.PlatformAPIBaseURL=https://platform.example.com
// 为空时按运行时配置自动解析（环境变量 / .pinchbot/config.json / 默认值）。
var PlatformAPIBaseURL = ""

func main() {
	app := NewApp(settingsURL, gatewayURL, PlatformAPIBaseURL)

	err := wails.Run(&options.App{
		Title:             title,
		Width:             windowWidth,
		Height:            windowHeight,
		MinWidth:          windowWidth,
		MinHeight:         windowHeight,
		DisableResize:     false,
		HideWindowOnClose: true,
		StartHidden:       false,
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		AssetServer: &assetserver.Options{
			Assets: frontendAssets,
		},
		Bind: []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
