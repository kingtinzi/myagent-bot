package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

// App 暴露给前端的 Go 方法（在 JS 里通过 window.go.xxx 调用）
type App struct {
	ctx         context.Context
	settingsURL string
	gatewayURL  string
}

func NewApp(settingsURL, gatewayURL string) *App {
	if gatewayURL == "" {
		gatewayURL = "http://127.0.0.1:18790"
	}
	return &App{settingsURL: settingsURL, gatewayURL: gatewayURL}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 主程序启动时自动拉起配置服务(picoclaw-launcher)与网关(picoclaw gateway)，不阻塞 UI
	go startBackendServices()
	// 后台检查更新（不阻塞）
	go func() {
		if res := a.CheckForUpdates(); res.Available != "" {
			log.Printf("[update] 发现新版本 %s，正在后台下载", res.Available)
		}
	}()
	// Windows 上 systray 必须在锁定的 OS 线程中运行，否则首次点击菜单后左/右键会失效（见 getlantern/systray#269）
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		systray.Run(func() { runTray(a) }, func() {})
	}()
}

func runTray(a *App) {
	if len(trayIcon) > 0 {
		systray.SetIcon(trayIcon)
	}
	systray.SetTooltip("PicoClaw 助理")
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
			if HasPendingUpdate() && runtime.GOOS == "windows" {
				RunApplyScriptAndExit()
			}
			wailsruntime.Quit(a.ctx)
		}
	}()
}

// GetVersion 返回当前版本号（构建时注入，用于关于页/更新检查）
func (a *App) GetVersion() string {
	return Version
}

// CheckUpdateResult 供前端展示的检查结果
type CheckUpdateResult struct {
	Current   string `json:"current"`
	Available string `json:"available,omitempty"`
	URL       string `json:"url,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Downloaded bool  `json:"downloaded,omitempty"`
	Error     string `json:"error,omitempty"`
}

// CheckForUpdates 检查是否有新版本；若有则在后台下载，下次启动即应用
func (a *App) CheckForUpdates() CheckUpdateResult {
	res := CheckUpdateResult{Current: Version}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m, err := FetchManifest(ctx)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if !versionLess(Version, m.Version) {
		return res
	}
	res.Available = m.Version
	res.URL = m.URL
	res.Notes = m.Notes
	// 后台下载
	go func() {
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer dlCancel()
		if _, err := DownloadUpdate(dlCtx, m); err != nil {
			log.Printf("[update] 下载失败: %v", err)
			return
		}
		log.Printf("[update] 新版本 %s 已下载，下次启动将自动应用", m.Version)
	}()
	return res
}

// HasPendingUpdate 是否已有下载好的更新待下次启动应用
func (a *App) HasPendingUpdate() bool {
	return HasPendingUpdate()
}

// OpenSettings 在默认浏览器打开配置页（如 http://localhost:18800）
func (a *App) OpenSettings() {
	openBrowser(a.settingsURL)
}

// SavePastedImage 将粘贴的图片（data URL 或纯 base64）写入临时文件，返回本地路径，供聊天附件发送
func (a *App) SavePastedImage(dataURLOrBase64 string) (string, error) {
	ext := ".png"
	var raw []byte
	if dataURLOrBase64 == "" {
		return "", fmt.Errorf("empty image data")
	}
	if len(dataURLOrBase64) > 10 && dataURLOrBase64[:5] == "data:" {
		re := regexp.MustCompile(`^data:image/(\w+);base64,`)
		if m := re.FindStringSubmatch(dataURLOrBase64); len(m) >= 2 {
			switch m[1] {
			case "jpeg", "jpg":
				ext = ".jpg"
			case "gif":
				ext = ".gif"
			case "webp":
				ext = ".webp"
			default:
				ext = ".png"
			}
		}
		if i := bytes.IndexByte([]byte(dataURLOrBase64), ','); i >= 0 {
			dataURLOrBase64 = dataURLOrBase64[i+1:]
		}
	}
	var err error
	raw, err = base64.StdEncoding.DecodeString(dataURLOrBase64)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	dir := filepath.Join(os.TempDir(), "picoclaw-paste")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("paste-%d%s", time.Now().UnixNano(), ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// SelectLocalFiles 打开本地文件选择对话框，返回选中的文件路径列表（用于聊天附件）
func (a *App) SelectLocalFiles() ([]string, error) {
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择要发送的文件",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "文档与表格", Pattern: "*.pdf;*.doc;*.docx;*.xls;*.xlsx;*.ppt;*.pptx;*.txt;*.md"},
			{DisplayName: "所有文件", Pattern: "*"},
		},
	})
}

// Chat 发送一条消息到 PicoClaw Gateway /api/chat，可选附带本地文件路径，返回 agent 回复
func (a *App) Chat(message string, attachments []string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"message":     message,
		"attachments": attachments,
	})
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, a.gatewayURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gateway 返回 %d，请确认 PicoClaw gateway 已启动（端口 18790）", resp.StatusCode)
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	_ = err
}

// startBackendServices 在后台启动 picoclaw-launcher（配置页 18800）与 picoclaw gateway（18790）。
// 查找顺序：与 launcher-chat 同目录的 picoclaw-launcher[.exe]、picoclaw[.exe]（或 picoclaw-windows-amd64.exe）；
// 若不存在则尝试 PicoClaw/build/（便于开发时与 Makefile 产物一起用）。
func startBackendServices() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[launcher] 无法获取可执行文件路径: %v", err)
		return
	}
	dir := filepath.Dir(exePath)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	tryPath := func(name string) string {
		p := filepath.Join(dir, name+suffix)
		if info, e := os.Stat(p); e == nil && !info.IsDir() {
			return p
		}
		return ""
	}

	// 1) 同目录：picoclaw-launcher[.exe]、picoclaw[.exe]
	launcherExe := tryPath("picoclaw-launcher")
	gatewayExe := tryPath("picoclaw")
	// 2) 同目录：Makefile 产物 picoclaw-windows-amd64.exe（仅 Windows）
	if gatewayExe == "" && runtime.GOOS == "windows" {
		gatewayExe = tryPath("picoclaw-windows-amd64")
	}
	// 3) 仓库内 PicoClaw/build/（从 dist/OpenClaw-xxx 或 Launcher/app-wails 回退到仓库根再进 PicoClaw/build）
	if launcherExe == "" || gatewayExe == "" {
		buildDir := filepath.Join(dir, "..", "..", "PicoClaw", "build")
		if runtime.GOOS == "windows" {
			if launcherExe == "" {
				p := filepath.Join(buildDir, "picoclaw-launcher.exe")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					launcherExe = p
				}
			}
			if gatewayExe == "" {
				for _, name := range []string{"picoclaw-windows-amd64.exe", "picoclaw.exe"} {
					p := filepath.Join(buildDir, name)
					if info, e := os.Stat(p); e == nil && !info.IsDir() {
						gatewayExe = p
						break
					}
				}
			}
		} else {
			if launcherExe == "" {
				p := filepath.Join(buildDir, "picoclaw-launcher")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					launcherExe = p
				}
			}
			if gatewayExe == "" {
				p := filepath.Join(buildDir, "picoclaw")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					gatewayExe = p
				}
			}
		}
	}

	if launcherExe != "" {
		cmd := exec.Command(launcherExe)
		cmd.Dir = filepath.Dir(launcherExe)
		setNoWindow(cmd)
		if err := cmd.Start(); err != nil {
			log.Printf("[launcher] 启动 picoclaw-launcher 失败: %v", err)
		} else {
			log.Printf("[launcher] 已启动配置服务: %s", launcherExe)
		}
	} else {
		log.Printf("[launcher] 未找到 picoclaw-launcher，请将本程序与 picoclaw-launcher 放在同一目录或使用 PicoClaw/build/")
	}

	if gatewayExe != "" {
		cmd := exec.Command(gatewayExe, "gateway")
		cmd.Dir = filepath.Dir(gatewayExe)
		setNoWindow(cmd)
		if err := cmd.Start(); err != nil {
			log.Printf("[launcher] 启动 picoclaw gateway 失败: %v", err)
		} else {
			log.Printf("[launcher] 已启动网关: %s gateway", gatewayExe)
		}
	} else {
		log.Printf("[launcher] 未找到 picoclaw(gateway)，请将本程序与 picoclaw 放在同一目录或使用 PicoClaw/build/")
	}
}
