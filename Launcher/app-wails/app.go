package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	ctx                       context.Context
	settingsURL               string
	gatewayURL                string
	platformMu                sync.Mutex
	platformPinned            bool
	platformRefreshFromConfig bool
	platformURL               string
	platformClient            *platformapi.Client
	sessionStore              *platformapi.FileSessionStore

	openBrowserFn               func(string)
	ensureGatewayServiceFn      func() error
	ensurePlatformServiceFn     func() error
	ensureSettingsServiceFn     func() error
	resolvePlatformExecutableFn func() (string, error)
	statFn                      func(string) (os.FileInfo, error)

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

var errDesktopSessionExpired = errors.New("session expired")

type OfficialPanelSnapshot struct {
	Access platformapi.OfficialAccessState `json:"access"`
	Models []platformapi.OfficialModel     `json:"models"`
}

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
	platformPinned := strings.TrimSpace(platformURL) != ""
	platformURL = resolvePlatformURL(platformURL)
	app := &App{
		settingsURL:               settingsURL,
		gatewayURL:                gatewayURL,
		platformPinned:            platformPinned,
		platformRefreshFromConfig: !platformPinned,
		platformURL:               platformURL,
		platformClient:            platformapi.NewClient(platformURL),
		sessionStore:              platformapi.NewFileSessionStore(defaultSessionStoreDir()),
	}
	app.openBrowserFn = openBrowser
	app.ensureGatewayServiceFn = app.ensureGatewayServiceStarted
	app.ensurePlatformServiceFn = app.ensurePlatformServiceStarted
	app.ensureSettingsServiceFn = app.ensureSettingsServiceStarted
	app.resolvePlatformExecutableFn = resolvePlatformExecutable
	app.statFn = os.Stat
	return app
}

func resolvePlatformURL(platformURL string) string {
	if resolved := strings.TrimSpace(platformURL); resolved != "" {
		return resolved
	}
	if resolved := strings.TrimSpace(os.Getenv("PICOCLAW_PLATFORM_API_BASE_URL")); resolved != "" {
		return resolved
	}
	cfg, err := pconfig.LoadConfig(pconfig.GetConfigPath())
	if err == nil {
		if resolved := strings.TrimSpace(cfg.PlatformAPI.BaseURL); resolved != "" {
			return resolved
		}
	}
	return pconfig.DefaultConfig().PlatformAPI.BaseURL
}

func (a *App) currentPlatformClient() *platformapi.Client {
	a.platformMu.Lock()
	defer a.platformMu.Unlock()

	if !a.platformRefreshFromConfig || a.platformPinned {
		if a.platformClient == nil && strings.TrimSpace(a.platformURL) != "" {
			a.platformClient = platformapi.NewClient(a.platformURL)
		}
		if a.platformClient != nil && strings.TrimSpace(a.platformURL) == "" {
			a.platformURL = a.platformClient.BaseURL()
		}
		return a.platformClient
	}

	resolvedURL := resolvePlatformURL("")
	if a.platformClient == nil || !strings.EqualFold(strings.TrimSpace(a.platformURL), strings.TrimSpace(resolvedURL)) {
		a.platformURL = resolvedURL
		a.platformClient = platformapi.NewClient(resolvedURL)
	}

	return a.platformClient
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
	Authenticated        bool   `json:"authenticated"`
	UserID               string `json:"user_id,omitempty"`
	Username             string `json:"username,omitempty"`
	Email                string `json:"email,omitempty"`
	BalanceFen           int64  `json:"balance_fen,omitempty"`
	Currency             string `json:"currency,omitempty"`
	Error                string `json:"error,omitempty"`
	Warning              string `json:"warning,omitempty"`
	AgreementSyncPending bool   `json:"agreement_sync_pending,omitempty"`
}

type authSessionResult struct {
	Session platformapi.Session
	Warning string
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
	client := a.currentPlatformClient()
	state := AuthState{
		Authenticated:        true,
		UserID:               session.UserID,
		Username:             session.Username,
		Email:                session.Email,
		Warning:              session.Warning,
		AgreementSyncPending: session.AgreementSyncPending,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wallet, err := client.GetWallet(ctx, session.AccessToken)
	if err == nil {
		state.BalanceFen = wallet.BalanceFen
		state.Currency = wallet.Currency
	} else {
		if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
			_ = a.sessionStore.Clear()
			return AuthState{}
		}
		state.Error = a.userFacingPlatformError(err)
	}
	return state
}

func (a *App) GetOfficialAccessState() (platformapi.OfficialAccessState, error) {
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return platformapi.OfficialAccessState{}, err
	}
	session, err := a.loadActivePlatformSession()
	if err != nil {
		return platformapi.OfficialAccessState{}, err
	}
	client := a.currentPlatformClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := client.GetOfficialAccessState(ctx, session.AccessToken)
	if err != nil {
		return platformapi.OfficialAccessState{}, a.normalizePlatformSessionError(err)
	}
	return state, nil
}

func (a *App) ListOfficialModels() ([]platformapi.OfficialModel, error) {
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return nil, err
	}
	session, err := a.loadActivePlatformSession()
	if err != nil {
		return nil, err
	}
	client := a.currentPlatformClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models, err := client.ListOfficialModels(ctx, session.AccessToken)
	if err != nil {
		return nil, a.normalizePlatformSessionError(err)
	}
	if err := a.syncOfficialModelsIntoDesktopConfig(models); err != nil {
		log.Printf("[launcher] 同步官方模型到本地配置失败: %v", err)
	}
	return models, nil
}

func (a *App) GetOfficialPanelSnapshot() (OfficialPanelSnapshot, error) {
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return OfficialPanelSnapshot{}, err
	}
	session, err := a.loadActivePlatformSession()
	if err != nil {
		return OfficialPanelSnapshot{}, err
	}
	client := a.currentPlatformClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	access, err := client.GetOfficialAccessState(ctx, session.AccessToken)
	if err != nil {
		return OfficialPanelSnapshot{}, a.normalizePlatformSessionError(err)
	}
	models, err := client.ListOfficialModels(ctx, session.AccessToken)
	if err != nil {
		return OfficialPanelSnapshot{}, a.normalizePlatformSessionError(err)
	}
	if err := a.syncOfficialModelsIntoDesktopConfig(models); err != nil {
		log.Printf("[launcher] 同步官方模型到本地配置失败: %v", err)
	}
	return OfficialPanelSnapshot{
		Access: access,
		Models: models,
	}, nil
}

func (a *App) GetBackendStatus() platformapi.BackendStatus {
	a.currentPlatformClient()
	return platformapi.BackendStatus{
		GatewayURL:      a.gatewayURL,
		GatewayHealthy:  serviceHealthy(gatewayReadyURL(a.gatewayURL)),
		PlatformURL:     a.platformURL,
		PlatformHealthy: serviceHealthy(a.platformURL + "/health"),
		SettingsURL:     a.settingsURL,
		SettingsHealthy: serviceHealthy(a.settingsURL + "/api/config"),
	}
}

func (a *App) ListAuthAgreements() ([]platformapi.AgreementDocument, error) {
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return nil, err
	}
	client := a.currentPlatformClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	docs, err := client.ListAgreements(ctx, "")
	if err != nil {
		return nil, a.normalizePlatformBootstrapError(err)
	}
	return platformapi.FilterAuthAgreements(docs), nil
}

func (a *App) SignIn(email, password string) (AuthState, error) {
	result, err := a.authenticateSession(platformapi.AuthRequest{Email: email, Password: password}, true)
	if err != nil {
		return AuthState{}, err
	}
	state := a.GetAuthState()
	state.Warning = result.Warning
	state.AgreementSyncPending = result.Session.AgreementSyncPending
	return state, nil
}

func (a *App) SignUp(email, password, username string) (AuthState, error) {
	return a.SignUpWithAgreements(email, password, username, nil)
}

func (a *App) SignOut() error {
	return a.sessionStore.Clear()
}

func (a *App) SignUpWithAgreements(email, password, username string, agreements []platformapi.AgreementDocument) (AuthState, error) {
	result, err := a.authenticateSession(platformapi.AuthRequest{
		Username:   strings.TrimSpace(username),
		Email:      email,
		Password:   password,
		Agreements: platformapi.FilterAuthAgreements(agreements),
	}, false)
	if err != nil {
		return AuthState{}, err
	}
	state := a.GetAuthState()
	state.Warning = result.Warning
	state.AgreementSyncPending = result.Session.AgreementSyncPending
	return state, nil
}

func (a *App) authenticateSession(req platformapi.AuthRequest, isLogin bool) (authSessionResult, error) {
	req.Email = strings.TrimSpace(req.Email)
	if !platformapi.IsLikelyValidEmailAddress(req.Email) {
		return authSessionResult{}, errors.New(platformapi.InvalidEmailFormatMessage)
	}
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return authSessionResult{}, err
	}
	client := a.currentPlatformClient()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var (
		resp platformapi.AuthResponse
		err  error
	)
	if isLogin {
		resp, err = client.LoginResponse(ctx, req)
	} else {
		resp, err = client.SignUpResponse(ctx, req)
	}
	if err != nil {
		return authSessionResult{}, a.normalizePlatformBootstrapError(err)
	}
	session := resp.Session
	session.AccessToken = strings.TrimSpace(session.AccessToken)
	if session.AccessToken == "" {
		return authSessionResult{}, errors.New(platformapi.NormalizeUserFacingErrorMessage("authentication service did not return a valid session"))
	}
	warning := platformapi.NormalizeUserFacingErrorMessage(resp.Warning)
	if !isLogin && len(req.Agreements) > 0 && resp.AgreementSyncRequired {
		if err := client.AcceptAgreements(ctx, session.AccessToken, platformapi.AcceptAgreementsRequest{
			Agreements: req.Agreements,
		}); err != nil {
			warning = "注册已成功，但协议确认同步失败，请在充值前重新确认协议"
			session.AgreementSyncPending = true
			session.Warning = warning
		} else {
			warning = ""
			session.AgreementSyncPending = false
			session.Warning = ""
		}
	}
	if err := a.sessionStore.Save(session); err != nil {
		return authSessionResult{}, err
	}
	if err := a.syncOfficialModelsForSession(ctx, session.AccessToken); err != nil {
		log.Printf("[launcher] 登录后同步官方模型失败: %v", err)
	}
	return authSessionResult{Session: session, Warning: warning}, nil
}

func (a *App) syncOfficialModelsForSession(ctx context.Context, accessToken string) error {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil
	}
	models, err := a.currentPlatformClient().ListOfficialModels(ctx, accessToken)
	if err != nil {
		return err
	}
	return a.syncOfficialModelsIntoDesktopConfig(models)
}

func (a *App) syncOfficialModelsIntoDesktopConfig(models []platformapi.OfficialModel) error {
	cfgPath := pconfig.GetConfigPath()
	cfg, err := pconfig.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	baseURL := strings.TrimSpace(cfg.PlatformAPI.BaseURL)
	if baseURL == "" {
		baseURL = a.currentPlatformClient().BaseURL()
	}
	if baseURL == "" {
		baseURL = pconfig.DefaultConfig().PlatformAPI.BaseURL
	}

	enabled := make(map[string]platformapi.OfficialModel, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || !model.Enabled {
			continue
		}
		enabled[model.ID] = model
	}

	defaultModel := cfg.Agents.Defaults.GetModelName()
	defaultRemoved := false
	out := make([]pconfig.ModelConfig, 0, len(cfg.ModelList)+len(enabled))
	seen := make(map[string]struct{}, len(enabled))
	preserveExistingOfficialModels := len(enabled) == 0
	imported := make([]string, 0, len(enabled))

	for _, item := range cfg.ModelList {
		modelID, isOfficial := desktopOfficialModelID(item.Model)
		if !isOfficial {
			out = append(out, item)
			continue
		}
		if preserveExistingOfficialModels {
			out = append(out, item)
			continue
		}
		model, ok := enabled[modelID]
		if !ok {
			if item.ModelName == defaultModel {
				defaultRemoved = true
			}
			continue
		}
		updated := item
		if strings.TrimSpace(updated.ModelName) == "" || strings.HasPrefix(strings.TrimSpace(updated.ModelName), "official-") {
			updated.ModelName = desktopOfficialModelAlias(model)
		}
		updated.Model = "official/" + model.ID
		updated.APIBase = baseURL
		updated.APIKey = ""
		updated.Proxy = ""
		out = append(out, updated)
		seen[model.ID] = struct{}{}
	}

	if preserveExistingOfficialModels {
		for _, existing := range out {
			if _, isOfficial := desktopOfficialModelID(existing.Model); isOfficial {
				imported = append(imported, existing.ModelName)
			}
		}
	} else {
		for _, model := range models {
			model.ID = strings.TrimSpace(model.ID)
			if model.ID == "" || !model.Enabled {
				continue
			}
			if _, ok := seen[model.ID]; ok {
				for _, existing := range out {
					if existing.Model == "official/"+model.ID {
						imported = append(imported, existing.ModelName)
						break
					}
				}
				continue
			}
			out = append(out, pconfig.ModelConfig{
				ModelName: desktopOfficialModelAlias(model),
				Model:     "official/" + model.ID,
				APIBase:   baseURL,
			})
			imported = append(imported, desktopOfficialModelAlias(model))
		}
	}

	cfg.ModelList = out
	if desktopShouldPromoteOfficialDefault(cfg, defaultModel, defaultRemoved, imported) {
		if len(imported) > 0 {
			cfg.Agents.Defaults.ModelName = imported[0]
		} else if len(out) > 0 {
			cfg.Agents.Defaults.ModelName = out[0].ModelName
		} else {
			cfg.Agents.Defaults.ModelName = ""
		}
	}
	return pconfig.SaveConfig(cfgPath, cfg)
}

func desktopOfficialModelID(model string) (string, bool) {
	protocol, modelID, found := strings.Cut(strings.TrimSpace(model), "/")
	if !found || protocol != "official" {
		return "", false
	}
	modelID = strings.TrimSpace(modelID)
	return modelID, modelID != ""
}

func desktopOfficialModelAlias(model platformapi.OfficialModel) string {
	label := strings.TrimSpace(model.ID)
	label = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(label)
	label = strings.ToLower(label)
	label = strings.Trim(label, "-")
	if label == "" {
		label = "model"
	}
	return fmt.Sprintf("official-%s", label)
}

func desktopShouldPromoteOfficialDefault(cfg *pconfig.Config, defaultModel string, defaultRemoved bool, imported []string) bool {
	if defaultRemoved || strings.TrimSpace(defaultModel) == "" {
		return true
	}
	if len(imported) == 0 || cfg == nil {
		return false
	}
	current, err := cfg.GetModelConfig(defaultModel)
	if err != nil || current == nil {
		return true
	}
	if _, isOfficial := desktopOfficialModelID(current.Model); isOfficial {
		return false
	}
	return desktopIsBootstrapSampleModel(*current)
}

func desktopIsBootstrapSampleModel(item pconfig.ModelConfig) bool {
	if strings.TrimSpace(item.AuthMethod) != "" {
		return false
	}
	apiKey := strings.TrimSpace(item.APIKey)
	if apiKey != "" && desktopLooksLikePlaceholderSecret(apiKey) {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(item.Model))
	apiBase := strings.ToLower(strings.TrimSpace(item.APIBase))
	switch model {
	case "openai/gpt-5.2":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.openai.com/v1")
	case "anthropic/claude-sonnet-4.6":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.anthropic.com/v1")
	case "deepseek/deepseek-chat":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.deepseek.com/v1")
	case "qwen/qwen-plus":
		return apiKey == "" && (apiBase == "" || apiBase == "https://dashscope.aliyuncs.com/compatible-mode/v1")
	default:
		return false
	}
}

func desktopLooksLikePlaceholderSecret(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return true
	}
	placeholders := []string{
		"sk-your-openai-key",
		"sk-ant-your-key",
		"your_dashscope_key",
		"your-dashscope-key",
		"replace-with-your-upstream-api-key",
		"your_api_key",
		"your-api-key",
		"gsk_xxx",
		"sk-xxx",
	}
	for _, placeholder := range placeholders {
		if value == placeholder {
			return true
		}
	}
	return strings.Contains(value, "your-key") || strings.Contains(value, "your_api_key")
}

func (a *App) loadActivePlatformSession() (platformapi.Session, error) {
	session, err := a.sessionStore.Load()
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "session file is incomplete") {
			return platformapi.Session{}, errDesktopSessionExpired
		}
		return platformapi.Session{}, err
	}
	if session.IsExpired(time.Now()) {
		_ = a.sessionStore.Clear()
		return platformapi.Session{}, errDesktopSessionExpired
	}
	return session, nil
}

func (a *App) normalizePlatformSessionError(err error) error {
	if err == nil {
		return nil
	}
	if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
		_ = a.sessionStore.Clear()
		return errDesktopSessionExpired
	}
	return a.normalizePlatformBootstrapError(err)
}

func (a *App) normalizePlatformBootstrapError(err error) error {
	if err == nil {
		return nil
	}
	if message := strings.TrimSpace(platformapi.UserFacingErrorMessage(err)); message != "" && message != strings.TrimSpace(err.Error()) {
		return errors.New(message)
	}
	client := a.currentPlatformClient()
	if !isLocalPlatformBaseURL(client.BaseURL()) || !isLikelyConnectionRefusedError(err) {
		if _, ok := err.(*platformapi.APIError); ok {
			return errors.New(platformapi.UserFacingErrorMessage(err))
		}
		return err
	}
	if a.resolvePlatformExecutableFn != nil {
		if exePath, resolveErr := a.resolvePlatformExecutableFn(); resolveErr == nil {
			statFn := a.statFn
			if statFn == nil {
				statFn = os.Stat
			}
			if !hasLivePlatformConfig(statFn, exePath) {
				return fmt.Errorf("本地平台注册服务尚未配置，请先在 config/platform.env 中填写平台配置后重新启动应用")
			}
		}
	}
	return fmt.Errorf("本地平台注册服务不可用，请检查 platform-server 是否已启动")
}

func (a *App) userFacingPlatformError(err error) string {
	if err == nil {
		return ""
	}
	return a.normalizePlatformBootstrapError(err).Error()
}

func isLocalPlatformBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func isLikelyConnectionRefusedError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"connection refused",
		"actively refused",
		"connectex",
		"dial tcp",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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
	if err := a.ensurePlatformServiceAvailable(); err != nil {
		return "", err
	}
	if err := a.ensureGatewayServiceAvailable(); err != nil {
		return "", err
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
		return "", fmt.Errorf("本地聊天网关返回 %d，请确认 PinchBot 聊天服务已启动（端口 18790）", resp.StatusCode)
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
	readyURL := gatewayReadyURL(a.gatewayURL)
	if serviceHealthy(readyURL) {
		return nil
	}
	if a.managedProcessRunning("gateway") {
		return waitForService(readyURL, 10*time.Second)
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
	return waitForService(readyURL, 10*time.Second)
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

func gatewayReadyURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/ready"
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

func (a *App) ensurePlatformServiceAvailable() error {
	if a.ensurePlatformServiceFn == nil {
		return nil
	}
	if err := a.ensurePlatformServiceFn(); err != nil {
		return a.normalizePlatformBootstrapError(err)
	}
	return nil
}

func (a *App) ensureGatewayServiceAvailable() error {
	if a.ensureGatewayServiceFn == nil {
		return nil
	}
	return a.ensureGatewayServiceFn()
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
			"settings service uses PINCHBOT_HOME %s but expected %s; please stop the existing launcher service and try again",
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
	legacyHome := filepath.Dir(cleanPath)
	if !samePath(legacyHome, defaultSessionStoreDir()) {
		return ""
	}
	return legacyHome
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
