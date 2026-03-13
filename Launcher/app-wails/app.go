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
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	pconfig "github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

// App 暴露给前端的 Go 方法（在 JS 里通过 window.go.xxx 调用）
type App struct {
	ctx            context.Context
	settingsURL    string
	gatewayURL     string
	platformURL    string
	platformClient *platformapi.Client
	sessionStore   *platformapi.FileSessionStore

	openBrowserFn           func(string)
	ensureGatewayServiceFn  func() error
	ensurePlatformServiceFn func() error
	ensureSettingsServiceFn func() error

	processMu    sync.Mutex
	shutdownOnce sync.Once
	gatewayProc  *managedProcess
	launcherProc *managedProcess
	platformProc *managedProcess

	gatewayLogMu    sync.Mutex
	gatewayLogLines []string
	gatewayLogRunID int
}

const authRequiredErrorPrefix = "AUTH_REQUIRED:"

type managedProcess struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
}

type settingsConfigResponse struct {
	Path string `json:"path"`
	Home string `json:"home"`
}

func NewApp(settingsURL, gatewayURL, platformURL string) *App {
	if gatewayURL == "" {
		gatewayURL = "http://127.0.0.1:18790"
	}
	if platformURL == "" {
		platformURL = "http://127.0.0.1:18791"
	}
	app := &App{
		settingsURL:    settingsURL,
		gatewayURL:     gatewayURL,
		platformURL:    platformURL,
		platformClient: platformapi.NewClient(platformURL),
		sessionStore:   platformapi.NewFileSessionStore(defaultSessionStoreDir()),
	}
	app.openBrowserFn = openBrowser
	app.ensureGatewayServiceFn = app.ensureGatewayServiceStarted
	app.ensurePlatformServiceFn = app.ensurePlatformServiceStarted
	app.ensureSettingsServiceFn = app.ensureSettingsServiceStarted
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 主程序启动时仅拉起网关与平台服务；设置页按需启动。
	go a.startManagedServices()
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
	Current    string `json:"current"`
	Available  string `json:"available,omitempty"`
	URL        string `json:"url,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Downloaded bool   `json:"downloaded,omitempty"`
	Error      string `json:"error,omitempty"`
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
	if a.ensureSettingsServiceFn != nil {
		if err := a.ensureSettingsServiceFn(); err != nil {
			log.Printf("[launcher] 启动 PinchBot 设置页失败: %v", err)
			return
		}
	}
	if a.openBrowserFn != nil {
		a.openBrowserFn(a.settingsURL)
	}
}

type AuthState struct {
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id,omitempty"`
	Email         string `json:"email,omitempty"`
	BalanceFen    int64  `json:"balance_fen,omitempty"`
	Currency      string `json:"currency,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (a *App) GetAuthState() AuthState {
	session, err := a.sessionStore.Load()
	if err != nil {
		return AuthState{}
	}
	if session.IsExpired(time.Now()) {
		_ = a.sessionStore.Clear()
		return AuthState{}
	}
	state := AuthState{
		Authenticated: true,
		UserID:        session.UserID,
		Email:         session.Email,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wallet, err := a.platformClient.GetWallet(ctx, session.AccessToken)
	if err == nil {
		state.BalanceFen = wallet.BalanceFen
		state.Currency = wallet.Currency
	} else {
		if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
			_ = a.sessionStore.Clear()
			return AuthState{}
		}
		state.Error = err.Error()
	}
	return state
}

func (a *App) GetOfficialAccessState() (platformapi.OfficialAccessState, error) {
	session, err := a.sessionStore.Load()
	if err != nil {
		return platformapi.OfficialAccessState{}, err
	}
	if session.IsExpired(time.Now()) {
		_ = a.sessionStore.Clear()
		return platformapi.OfficialAccessState{}, fmt.Errorf("session expired")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := a.platformClient.GetOfficialAccessState(ctx, session.AccessToken)
	if err != nil {
		if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
			_ = a.sessionStore.Clear()
		}
		return platformapi.OfficialAccessState{}, err
	}
	return state, nil
}

func (a *App) ListOfficialModels() ([]platformapi.OfficialModel, error) {
	session, err := a.sessionStore.Load()
	if err != nil {
		return nil, err
	}
	if session.IsExpired(time.Now()) {
		_ = a.sessionStore.Clear()
		return nil, fmt.Errorf("session expired")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models, err := a.platformClient.ListOfficialModels(ctx, session.AccessToken)
	if err != nil {
		if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
			_ = a.sessionStore.Clear()
		}
		return nil, err
	}
	return models, nil
}

func (a *App) GetBackendStatus() map[string]any {
	return map[string]any{
		"gateway_url":      a.gatewayURL,
		"gateway_healthy":  serviceHealthy(a.gatewayURL + "/health"),
		"platform_url":     a.platformURL,
		"platform_healthy": serviceHealthy(a.platformURL + "/health"),
		"settings_url":     a.settingsURL,
		"settings_healthy": serviceHealthy(a.settingsURL + "/api/config"),
	}
}

func (a *App) SignIn(email, password string) (AuthState, error) {
	return a.authenticate(platformapi.AuthRequest{Email: email, Password: password}, true)
}

func (a *App) SignUp(email, password string) (AuthState, error) {
	return a.authenticate(platformapi.AuthRequest{Email: email, Password: password}, false)
}

func (a *App) SignOut() error {
	return a.sessionStore.Clear()
}

func (a *App) authenticate(req platformapi.AuthRequest, isLogin bool) (AuthState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var (
		session platformapi.Session
		err     error
	)
	if isLogin {
		session, err = a.platformClient.Login(ctx, req)
	} else {
		session, err = a.platformClient.SignUp(ctx, req)
	}
	if err != nil {
		return AuthState{}, err
	}
	if err := a.sessionStore.Save(session); err != nil {
		return AuthState{}, err
	}
	return a.GetAuthState(), nil
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

// Chat 发送一条消息到 PinchBot Gateway /api/chat，可选附带本地文件路径，返回 agent 回复
func (a *App) Chat(message string, attachments []string) (string, error) {
	session, err := a.sessionStore.Load()
	if err != nil {
		return "", fmt.Errorf("%s%s", authRequiredErrorPrefix, "请先登录后再开始聊天")
	}
	if session.IsExpired(time.Now()) {
		_ = a.sessionStore.Clear()
		return "", fmt.Errorf("%s%s", authRequiredErrorPrefix, "登录状态已过期，请重新登录")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"message":     message,
		"attachments": attachments,
	})
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, a.gatewayURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		_ = a.sessionStore.Clear()
		return "", fmt.Errorf("%s%s", authRequiredErrorPrefix, "登录状态已过期，请重新登录")
	}
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

func (a *App) startManagedServices() {
	if a.ensureGatewayServiceFn != nil {
		if err := a.ensureGatewayServiceFn(); err != nil {
			log.Printf("[launcher] 启动 PinchBot 网关失败: %v", err)
		}
	}
	if a.ensurePlatformServiceFn != nil {
		if err := a.ensurePlatformServiceFn(); err != nil {
			log.Printf("[launcher] 启动平台服务失败: %v", err)
		}
	}
}

func (a *App) shutdown(context.Context) {
	a.shutdownOnce.Do(func() {
		systray.Quit()
		a.stopManagedServices()
	})
}

func (a *App) ensureGatewayServiceStarted() error {
	if serviceHealthy(a.gatewayURL + "/health") {
		return nil
	}
	if a.managedProcessRunning("gateway") {
		return waitForService(a.gatewayURL+"/health", 10*time.Second)
	}
	exePath, err := resolveGatewayExecutable()
	if err != nil {
		return err
	}
	proc, err := a.startManagedProcess("gateway", exePath, []string{"gateway"}, true)
	if err != nil {
		return err
	}
	a.setManagedProcess("gateway", proc)
	if serviceHealthy(a.settingsURL + "/api/config") {
		if err := a.initializeGatewayLogRelay(); err != nil {
			log.Printf("[launcher] 初始化网关日志转发失败: %v", err)
		}
	}
	return waitForService(a.gatewayURL+"/health", 10*time.Second)
}

func (a *App) ensurePlatformServiceStarted() error {
	exePath, err := resolvePlatformExecutable()
	if err != nil {
		log.Printf("[launcher] 未找到 platform-server；官方模型、钱包与充值能力将不可用")
		return nil
	}
	if !hasLivePlatformConfig(os.Stat, exePath) {
		log.Printf("[launcher] 跳过 platform-server 自动启动；请先提供 live 配置: %s", platformConfigPath(exePath))
		return nil
	}
	if serviceHealthy(a.platformURL + "/health") {
		return nil
	}
	if a.managedProcessRunning("platform") {
		return waitForService(a.platformURL+"/health", 10*time.Second)
	}
	proc, err := a.startManagedProcess("platform", exePath, nil, false)
	if err != nil {
		return err
	}
	a.setManagedProcess("platform", proc)
	return waitForService(a.platformURL+"/health", 10*time.Second)
}

func (a *App) ensureSettingsServiceStarted() error {
	if serviceHealthy(a.settingsURL + "/api/config") {
		if err := ensureSettingsServiceMatchesHome(a.settingsURL); err != nil {
			return err
		}
		if err := a.initializeGatewayLogRelay(); err != nil {
			log.Printf("[launcher] 初始化网关日志转发失败: %v", err)
		}
		return nil
	}
	if a.managedProcessRunning("settings") {
		if err := waitForService(a.settingsURL+"/api/config", 10*time.Second); err != nil {
			return err
		}
		if err := ensureSettingsServiceMatchesHome(a.settingsURL); err != nil {
			return err
		}
		if err := a.initializeGatewayLogRelay(); err != nil {
			log.Printf("[launcher] 初始化网关日志转发失败: %v", err)
		}
		return nil
	}
	exePath, err := resolveSettingsExecutable()
	if err != nil {
		return err
	}
	proc, err := a.startManagedProcess("settings", exePath, nil, false)
	if err != nil {
		return err
	}
	a.setManagedProcess("settings", proc)
	if err := waitForService(a.settingsURL+"/api/config", 10*time.Second); err != nil {
		return err
	}
	if err := ensureSettingsServiceMatchesHome(a.settingsURL); err != nil {
		return err
	}
	if err := a.initializeGatewayLogRelay(); err != nil {
		log.Printf("[launcher] 初始化网关日志转发失败: %v", err)
	}
	return nil
}

func (a *App) startManagedProcess(name, exePath string, args []string, captureGatewayLogs bool) (*managedProcess, error) {
	workdir := serviceWorkingDir(exePath)
	cmd := exec.Command(exePath, args...)
	cmd.Dir = workdir
	cmd.Env = serviceProcessEnv()
	setNoWindow(cmd)

	var stdout io.ReadCloser
	var stderr io.ReadCloser
	var err error
	if captureGatewayLogs {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		stderr, err = cmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		a.resetGatewayLogs()
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	proc := &managedProcess{
		name: name,
		cmd:  cmd,
		done: make(chan struct{}),
	}

	if captureGatewayLogs {
		go scanManagedOutput(stdout, a.appendGatewayLogLine)
		go scanManagedOutput(stderr, a.appendGatewayLogLine)
		a.appendGatewayLogLine(fmt.Sprintf("[launcher] Started gateway (PID: %d) from %s", cmd.Process.Pid, exePath))
	}

	go func() {
		defer close(proc.done)
		if err := cmd.Wait(); err != nil {
			log.Printf("[launcher] %s process exited: %v", name, err)
		}
		a.clearManagedProcess(name, proc)
	}()

	return proc, nil
}

func (a *App) setManagedProcess(name string, proc *managedProcess) {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	switch name {
	case "gateway":
		a.gatewayProc = proc
	case "settings":
		a.launcherProc = proc
	case "platform":
		a.platformProc = proc
	}
}

func (a *App) managedProcessRunning(name string) bool {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	switch name {
	case "gateway":
		return a.gatewayProc != nil
	case "settings":
		return a.launcherProc != nil
	case "platform":
		return a.platformProc != nil
	default:
		return false
	}
}

func (a *App) clearManagedProcess(name string, proc *managedProcess) {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	switch name {
	case "gateway":
		if a.gatewayProc == proc {
			a.gatewayProc = nil
		}
	case "settings":
		if a.launcherProc == proc {
			a.launcherProc = nil
		}
	case "platform":
		if a.platformProc == proc {
			a.platformProc = nil
		}
	}
}

func (a *App) stopManagedServices() {
	procs := make([]*managedProcess, 0, 3)
	a.processMu.Lock()
	for _, proc := range []*managedProcess{a.launcherProc, a.platformProc, a.gatewayProc} {
		if proc != nil {
			procs = append(procs, proc)
		}
	}
	a.launcherProc = nil
	a.platformProc = nil
	a.gatewayProc = nil
	a.processMu.Unlock()

	for _, proc := range procs {
		stopManagedProcess(proc)
	}
}

func (a *App) resetGatewayLogs() {
	a.gatewayLogMu.Lock()
	defer a.gatewayLogMu.Unlock()
	a.gatewayLogLines = nil
	a.gatewayLogRunID = 0
}

func (a *App) appendGatewayLogLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	a.gatewayLogMu.Lock()
	a.gatewayLogLines = append(a.gatewayLogLines, line)
	if len(a.gatewayLogLines) > 200 {
		a.gatewayLogLines = append([]string(nil), a.gatewayLogLines[len(a.gatewayLogLines)-200:]...)
	}
	runID := a.gatewayLogRunID
	a.gatewayLogMu.Unlock()

	if runID != 0 {
		if err := a.appendGatewayLogsRemote(runID, []string{line}); err != nil {
			log.Printf("[launcher] 追加网关日志失败: %v", err)
		}
	}
}

func (a *App) initializeGatewayLogRelay() error {
	runID, err := a.startGatewayLogSessionRemote()
	if err != nil {
		return err
	}

	a.gatewayLogMu.Lock()
	a.gatewayLogRunID = runID
	lines := append([]string(nil), a.gatewayLogLines...)
	a.gatewayLogMu.Unlock()

	if len(lines) > 0 {
		return a.appendGatewayLogsRemote(runID, lines)
	}
	return nil
}

func (a *App) startGatewayLogSessionRemote() (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, a.settingsURL+"/api/process/logs/start", nil)
	if err != nil {
		return 0, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("settings log start returned %d", resp.StatusCode)
	}
	var payload struct {
		RunID int `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}
	if payload.RunID == 0 {
		return 0, fmt.Errorf("settings log start returned empty run id")
	}
	return payload.RunID, nil
}

func (a *App) appendGatewayLogsRemote(runID int, lines []string) error {
	body, err := json.Marshal(map[string]any{
		"run_id": runID,
		"lines":  lines,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, a.settingsURL+"/api/process/logs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("settings log append returned %d", resp.StatusCode)
	}
	return nil
}

func scanManagedOutput(r io.Reader, appendLine func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		appendLine(scanner.Text())
	}
}

func stopManagedProcess(proc *managedProcess) {
	if proc == nil || proc.cmd == nil || proc.cmd.Process == nil {
		return
	}
	_ = proc.cmd.Process.Kill()
	select {
	case <-proc.done:
	case <-time.After(5 * time.Second):
		log.Printf("[launcher] wait for %s shutdown timed out", proc.name)
	}
}

func serviceHealthy(url string) bool {
	resp, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitForService(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if serviceHealthy(url) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service %s did not become ready within %s", url, timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
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

func defaultSessionStoreDir() string {
	return pconfig.GetPinchBotHome()
}

func serviceProcessEnv() []string {
	return serviceProcessEnvWithBase(os.Environ())
}

func serviceProcessEnvWithBase(base []string) []string {
	return upsertEnv(base, pconfig.PinchBotHomeEnv, defaultSessionStoreDir())
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := append([]string(nil), env...)
	for i, entry := range out {
		if strings.HasPrefix(entry, prefix) {
			out[i] = prefix + value
			return out
		}
	}
	return append(out, prefix+value)
}

func ensureSettingsServiceMatchesHome(settingsURL string) error {
	runtimeHome, err := fetchSettingsServiceHome(settingsURL)
	if err != nil {
		return err
	}
	expectedHome := defaultSessionStoreDir()
	if !samePath(runtimeHome, expectedHome) {
		return fmt.Errorf(
			"settings service uses PINCHBOT_HOME %q, expected %q; please stop the existing launcher service and try again",
			runtimeHome,
			expectedHome,
		)
	}
	return nil
}

func fetchSettingsServiceHome(settingsURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, settingsURL+"/api/config", nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("settings config endpoint returned %d", resp.StatusCode)
	}
	var payload settingsConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode settings config response: %w", err)
	}
	if runtimeHome := strings.TrimSpace(payload.Home); runtimeHome != "" {
		return runtimeHome, nil
	}
	if legacyHome := inferLegacySettingsServiceHome(payload.Path); legacyHome != "" {
		return legacyHome, nil
	}
	return "", fmt.Errorf("settings service did not report its PINCHBOT_HOME")
}

func samePath(a, b string) bool {
	cleanA := filepath.Clean(strings.TrimSpace(a))
	cleanB := filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}

func inferLegacySettingsServiceHome(configPath string) string {
	cleanPath := filepath.Clean(strings.TrimSpace(configPath))
	if cleanPath == "." || cleanPath == "" {
		return ""
	}
	if !strings.EqualFold(filepath.Base(cleanPath), "config.json") {
		return ""
	}
	return filepath.Dir(cleanPath)
}

func serviceWorkingDir(exePath string) string {
	dir := filepath.Dir(exePath)
	if strings.EqualFold(filepath.Base(dir), "MacOS") {
		contentsDir := filepath.Dir(dir)
		appDir := filepath.Dir(contentsDir)
		if strings.EqualFold(filepath.Base(contentsDir), "Contents") && strings.HasSuffix(strings.ToLower(appDir), ".app") {
			return filepath.Dir(appDir)
		}
	}
	return dir
}

func platformConfigPath(exePath string) string {
	return filepath.Join(serviceWorkingDir(exePath), "config", "platform.env")
}

func hasLivePlatformConfig(stat func(string) (os.FileInfo, error), exePath string) bool {
	info, err := stat(platformConfigPath(exePath))
	return err == nil && !info.IsDir()
}

func resolveGatewayExecutable() (string, error) {
	names := []string{"pinchbot", "picoclaw"}
	if runtime.GOOS == "windows" {
		names = append(names, "picoclaw-windows-amd64")
	}
	return resolveCompanionExecutable(names, []string{filepath.Join("..", "..", "PinchBot", "build")})
}

func resolveSettingsExecutable() (string, error) {
	return resolveCompanionExecutable(
		[]string{"pinchbot-launcher", "picoclaw-launcher"},
		[]string{filepath.Join("..", "..", "PinchBot", "build")},
	)
}

func resolvePlatformExecutable() (string, error) {
	return resolveCompanionExecutable(
		[]string{"platform-server"},
		[]string{
			filepath.Join("..", "..", "Platform"),
			filepath.Join("..", "..", "PinchBot", "build"),
		},
	)
}

func resolveCompanionExecutable(names []string, fallbackDirs []string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	dir := filepath.Dir(exePath)
	rootDir := serviceWorkingDir(exePath)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	tryPath := func(baseDir, name string) string {
		p := filepath.Join(baseDir, name+suffix)
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			return p
		}
		return ""
	}

	for _, name := range names {
		if candidate := tryPath(dir, name); candidate != "" {
			return candidate, nil
		}
	}

	for _, relDir := range fallbackDirs {
		searchDir := filepath.Clean(filepath.Join(rootDir, relDir))
		for _, name := range names {
			if candidate := tryPath(searchDir, name); candidate != "" {
				return candidate, nil
			}
		}
	}

	for _, name := range names {
		if resolved, lookErr := exec.LookPath(name + suffix); lookErr == nil {
			return resolved, nil
		}
		if suffix == "" {
			if resolved, lookErr := exec.LookPath(name); lookErr == nil {
				return resolved, nil
			}
		}
	}

	return "", fmt.Errorf("unable to find companion executable; looked for %s", strings.Join(names, ", "))
}
