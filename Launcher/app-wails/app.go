package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

// backendProcs 保存由 startBackendServices 启动的子进程，退出时由 stopBackendServices 关闭。
var backendProcs struct {
	sync.Mutex
	launcher *os.Process
	gateway  *os.Process
}

// gatewayLogBuf 缓存当前运行的 PinchBot 网关的 stdout/stderr，供配置页在「设置」打开时展示。
const gatewayLogBufCap = 500

var gatewayLogBuf struct {
	mu        sync.Mutex
	lines     []string
	forwardURL string // 非空时，新行同时 POST 到该 URL（配置页）
}

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
	// 主程序启动时仅自动拉起网关(PinchBot)；配置页(PinchBot-launcher)在点击「设置」时按需启动
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
	systray.SetTooltip("PinchBot 助理")
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
			stopBackendServices()
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

// OpenSettings 打开配置页：若 PinchBot-launcher 未运行则先启动，再打开浏览器，并把当前网关日志绑定到配置页。
func (a *App) OpenSettings() {
	// 若配置页未在运行，先启动 PinchBot-launcher（配置看板按需启动）
	if !configServerReachable(a.settingsURL) {
		startConfigServerOnce()
		time.Sleep(2 * time.Second) // 等待 HTTP 监听
	}
	openBrowser(a.settingsURL)
	// 将当前网关日志缓冲推送到配置页并开启后续转发，使配置页显示「当前运行的 PinchBot」的输出
	startLogForwardToConfig(a.settingsURL)
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
	dir := filepath.Join(os.TempDir(), "PinchBot-paste")
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

// Chat 发送一条消息到 PinchBot Gateway /api/chat，可选附带本地文件路径，返回 agent 回复
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
		return "", fmt.Errorf("gateway 返回 %d，请确认 PinchBot gateway 已启动（端口 18790）", resp.StatusCode)
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

// startBackendServices 在后台仅启动 PinchBot gateway（18790）；网关输出写入内存缓冲，供配置页打开时展示。
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

	// 1) 同目录：PinchBot-launcher[.exe]、PinchBot[.exe]
	launcherExe := tryPath("PinchBot-launcher")
	gatewayExe := tryPath("PinchBot")
	// 2) 同目录：Makefile 产物 PinchBot-windows-amd64.exe（仅 Windows）
	if gatewayExe == "" && runtime.GOOS == "windows" {
		gatewayExe = tryPath("PinchBot-windows-amd64")
	}
	// 3) 仓库内 PinchBot/build/（从 dist/OpenClaw-xxx 或 Launcher/app-wails 回退到仓库根再进 PinchBot/build）
	if launcherExe == "" || gatewayExe == "" {
		buildDir := filepath.Join(dir, "..", "..", "PinchBot", "build")
		if runtime.GOOS == "windows" {
			if launcherExe == "" {
				p := filepath.Join(buildDir, "PinchBot-launcher.exe")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					launcherExe = p
				}
			}
			if gatewayExe == "" {
				for _, name := range []string{"PinchBot-windows-amd64.exe", "PinchBot.exe"} {
					p := filepath.Join(buildDir, name)
					if info, e := os.Stat(p); e == nil && !info.IsDir() {
						gatewayExe = p
						break
					}
				}
			}
		} else {
			if launcherExe == "" {
				p := filepath.Join(buildDir, "PinchBot-launcher")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					launcherExe = p
				}
			}
			if gatewayExe == "" {
				p := filepath.Join(buildDir, "PinchBot")
				if info, e := os.Stat(p); e == nil && !info.IsDir() {
					gatewayExe = p
				}
			}
		}
	}

	// PinchBot-launcher 不在此处启动，仅在被点击「设置」时由 OpenSettings 按需启动

	if gatewayExe != "" {
		cmd := exec.Command(gatewayExe, "gateway")
		cmd.Dir = filepath.Dir(gatewayExe)
		setNoWindow(cmd)
		stdoutPipe, errOut := cmd.StdoutPipe()
		stderrPipe, errErr := cmd.StderrPipe()
		if errOut != nil || errErr != nil {
			log.Printf("[launcher] 无法创建网关输出管道: %v %v", errOut, errErr)
		}
		if err := cmd.Start(); err != nil {
			log.Printf("[launcher] 启动 PinchBot gateway 失败: %v", err)
		} else {
			backendProcs.Lock()
			backendProcs.gateway = cmd.Process
			backendProcs.Unlock()
			log.Printf("[launcher] 已启动网关: %s gateway", gatewayExe)
			if errOut == nil && errErr == nil {
				go forwardGatewayLogsToBuffer(stdoutPipe)
				go forwardGatewayLogsToBuffer(stderrPipe)
			}
		}
	} else {
		log.Printf("[launcher] 未找到 PinchBot(gateway)，请将本程序与 PinchBot 放在同一目录或使用 PinchBot/build/")
	}
}

// appendGatewayLogLine 将一行追加到缓冲，若已开启转发则同时 POST 到配置页。
func appendGatewayLogLine(line string) {
	gatewayLogBuf.mu.Lock()
	if len(gatewayLogBuf.lines) >= gatewayLogBufCap {
		gatewayLogBuf.lines = gatewayLogBuf.lines[1:]
	}
	gatewayLogBuf.lines = append(gatewayLogBuf.lines, line)
	url := gatewayLogBuf.forwardURL
	gatewayLogBuf.mu.Unlock()
	if url != "" {
		body, _ := json.Marshal(map[string]interface{}{"lines": []string{line}})
		req, _ := http.NewRequest(http.MethodPost, url+"/api/process/logs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req); err == nil {
			res.Body.Close()
		}
	}
}

// forwardGatewayLogsToBuffer 从 r 按行读取并写入网关日志缓冲（并可选转发到配置页）。
func forwardGatewayLogsToBuffer(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		appendGatewayLogLine(scanner.Text())
	}
}

func configServerReachable(baseURL string) bool {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/process/status", nil)
	if err != nil {
		return false
	}
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

var startConfigServerOnce = func() func() {
	var once sync.Once
	run := func() {
		exePath, err := os.Executable()
		if err != nil {
			return
		}
		dir := filepath.Dir(exePath)
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		launcherExe := ""
		for _, name := range []string{"PinchBot-launcher"} {
			p := filepath.Join(dir, name+suffix)
			if info, e := os.Stat(p); e == nil && !info.IsDir() {
				launcherExe = p
				break
			}
		}
		if launcherExe == "" {
			buildDir := filepath.Join(dir, "..", "..", "PinchBot", "build")
			p := filepath.Join(buildDir, "PinchBot-launcher"+suffix)
			if info, e := os.Stat(p); e == nil && !info.IsDir() {
				launcherExe = p
			}
		}
		if launcherExe == "" {
			return
		}
		cmd := exec.Command(launcherExe)
		cmd.Dir = filepath.Dir(launcherExe)
		setNoWindow(cmd)
		if err := cmd.Start(); err != nil {
			log.Printf("[launcher] 启动 PinchBot-launcher 失败: %v", err)
			return
		}
		backendProcs.Lock()
		backendProcs.launcher = cmd.Process
		backendProcs.Unlock()
		log.Printf("[launcher] 已启动配置服务: %s", launcherExe)
	}
	return func() { once.Do(run) }
}()

// startLogForwardToConfig 将当前网关日志缓冲推送到配置页并开启后续实时转发。
func startLogForwardToConfig(baseURL string) {
	gatewayLogBuf.mu.Lock()
	gatewayLogBuf.forwardURL = baseURL
	lines := make([]string, len(gatewayLogBuf.lines))
	copy(lines, gatewayLogBuf.lines)
	gatewayLogBuf.mu.Unlock()

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/process/logs/start", nil)
	if res, err := client.Do(req); err == nil {
		res.Body.Close()
	}
	if len(lines) > 0 {
		body, _ := json.Marshal(map[string]interface{}{"lines": lines})
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/process/logs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if res, err := client.Do(req); err == nil {
			res.Body.Close()
		}
	}
}

// waitProcessExit 等待进程退出，最多等待 timeout，避免 exe 被占用导致 dist 无法删除。
func waitProcessExit(p *os.Process, timeout time.Duration) {
	if p == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = p.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("[launcher] 进程 %d 未在 %v 内退出，继续退出", p.Pid, timeout)
	}
}

// stopBackendServices 结束由 startBackendServices 启动的配置服务与网关进程（选择「退出」或应用关闭时调用）。
// 会等待子进程实际退出后再返回，确保 exe 释放文件句柄，用户可立即删除 dist 目录。
func stopBackendServices() {
	backendProcs.Lock()
	p1 := backendProcs.launcher
	p2 := backendProcs.gateway
	backendProcs.launcher = nil
	backendProcs.gateway = nil
	backendProcs.Unlock()

	const waitTimeout = 5 * time.Second
	if p1 != nil {
		_ = p1.Kill()
		log.Printf("[launcher] 已结束配置服务进程，等待退出…")
		waitProcessExit(p1, waitTimeout)
	}
	if p2 != nil {
		_ = p2.Kill()
		log.Printf("[launcher] 已结束网关进程，等待退出…")
		waitProcessExit(p2, waitTimeout)
	}
}
