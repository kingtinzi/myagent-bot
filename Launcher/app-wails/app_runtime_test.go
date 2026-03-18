package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	launcherui "github.com/sipeed/pinchbot/pkg/launcherui"
	pinchlogger "github.com/sipeed/pinchbot/pkg/logger"
	pconfig "github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func TestConfigureTrayAppearanceSetsPinchBotIconAndTooltip(t *testing.T) {
	originalIcon := trayIcon
	originalSetIcon := systraySetIcon
	originalSetTooltip := systraySetTooltip
	t.Cleanup(func() {
		trayIcon = originalIcon
		systraySetIcon = originalSetIcon
		systraySetTooltip = originalSetTooltip
	})

	var gotIcon []byte
	var gotTooltip string
	trayIcon = []byte{0x01, 0x02, 0x03}
	systraySetIcon = func(icon []byte) {
		gotIcon = append([]byte(nil), icon...)
	}
	systraySetTooltip = func(tooltip string) {
		gotTooltip = tooltip
	}

	configureTrayAppearance()

	if !reflect.DeepEqual(gotIcon, trayIcon) {
		t.Fatalf("configureTrayAppearance() icon = %#v, want %#v", gotIcon, trayIcon)
	}
	if gotTooltip != "PinchBot" {
		t.Fatalf("configureTrayAppearance() tooltip = %q, want %q", gotTooltip, "PinchBot")
	}
}

func TestOpenSettingsStartsLauncherBeforeOpeningBrowser(t *testing.T) {
	var calls []string
	app := &App{
		settingsURL: "http://127.0.0.1:18800",
		ensureSettingsServiceFn: func() error {
			calls = append(calls, "ensure")
			return nil
		},
		openBrowserFn: func(url string) {
			calls = append(calls, url)
		},
	}

	app.OpenSettings()

	want := []string{"ensure", "http://127.0.0.1:18800"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("OpenSettings() call order = %#v, want %#v", calls, want)
	}
}

func TestOpenSettingsSkipsBrowserWhenLauncherStartFails(t *testing.T) {
	app := &App{
		settingsURL: "http://127.0.0.1:18800",
		ensureSettingsServiceFn: func() error {
			return errors.New("launcher failed")
		},
		openBrowserFn: func(string) {
			t.Fatal("OpenSettings() should not open the browser when launcher startup fails")
		},
	}

	app.OpenSettings()
}

func TestStartManagedServicesDoesNotStartSettingsLauncher(t *testing.T) {
	var calls []string
	app := &App{
		ensureGatewayServiceFn: func() error {
			calls = append(calls, "gateway")
			return nil
		},
		ensurePlatformServiceFn: func() error {
			calls = append(calls, "platform")
			return nil
		},
		ensureSettingsServiceFn: func() error {
			calls = append(calls, "settings")
			return nil
		},
	}

	app.startManagedServices()

	want := []string{"gateway", "platform"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("startManagedServices() calls = %#v, want %#v", calls, want)
	}
}

func TestChatEnsuresPlatformAndGatewayServicesBeforeRequest(t *testing.T) {
	var calls []string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "ok"})
	}))
	defer gateway.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "token-1",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		ctx:        context.Background(),
		gatewayURL: gateway.URL,
		sessionStore: store,
		ensurePlatformServiceFn: func() error {
			calls = append(calls, "platform")
			return nil
		},
		ensureGatewayServiceFn: func() error {
			calls = append(calls, "gateway")
			return nil
		},
	}

	reply, err := app.Chat("hello", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply != "ok" {
		t.Fatalf("reply = %q, want %q", reply, "ok")
	}
	if !reflect.DeepEqual(calls, []string{"platform", "gateway"}) {
		t.Fatalf("service start calls = %#v, want platform then gateway", calls)
	}
}

func TestListAuthAgreementsEnsuresPlatformServiceBeforeRequest(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agreements/current" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/agreements/current")
		}
		_ = json.NewEncoder(w).Encode([]platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议"},
		})
	}))
	defer server.Close()

	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   platformapi.NewFileSessionStore(t.TempDir()),
		ensurePlatformServiceFn: func() error {
			calls = append(calls, "platform")
			return nil
		},
	}

	if _, err := app.ListAuthAgreements(); err != nil {
		t.Fatalf("ListAuthAgreements() error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"platform"}) {
		t.Fatalf("service start calls = %#v, want platform bootstrap before agreements request", calls)
	}
}

func TestGetBackendStatusUsesGatewayReadyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			http.Error(w, `{"status":"not ready"}`, http.StatusServiceUnavailable)
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/config":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"path":"config.json"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		gatewayURL:  server.URL,
		platformURL: server.URL,
		settingsURL: server.URL,
	}

	status := app.GetBackendStatus()
	if status.GatewayHealthy {
		t.Fatalf("GatewayHealthy = true, want false when /ready is unavailable")
	}
}

func TestEnsureSettingsServiceMatchesHomeAcceptsMatchingHome(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", homeDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/config")
		}
		_ = json.NewEncoder(w).Encode(settingsConfigResponse{
			Path: filepath.Join(homeDir, "config.json"),
			Home: homeDir,
		})
	}))
	defer server.Close()

	if err := ensureSettingsServiceMatchesHome(server.URL); err != nil {
		t.Fatalf("ensureSettingsServiceMatchesHome() error = %v", err)
	}
}

func TestEnsureSettingsServiceStartedRunsEmbeddedServerWhenPortIsFree(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", homeDir)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	app := &App{
		settingsURL: "http://" + addr,
		settingsHandlerFn: func() (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/config" {
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(settingsConfigResponse{
					Path: filepath.Join(homeDir, "config.json"),
					Home: homeDir,
				})
			}), nil
		},
		settingsListenFn: func(network, address string) (net.Listener, error) {
			return net.Listen(network, address)
		},
	}
	t.Cleanup(func() {
		app.stopEmbeddedSettingsService()
	})

	if err := app.ensureSettingsServiceStarted(); err != nil {
		t.Fatalf("ensureSettingsServiceStarted() error = %v", err)
	}

	resp, err := http.Get(app.settingsURL + "/api/config")
	if err != nil {
		t.Fatalf("GET settings /api/config error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET settings /api/config status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestEnsureSettingsServiceStartedServesRealLauncherRoutesInProcess(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", homeDir)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	cfgPath := filepath.Join(homeDir, "config.json")
	if err := pconfig.SaveConfig(cfgPath, pconfig.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	app := &App{
		settingsURL: "http://" + addr,
		settingsHandlerFn: func() (http.Handler, error) {
			return launcherui.NewHandler(cfgPath)
		},
		settingsListenFn: net.Listen,
	}
	t.Cleanup(func() {
		app.stopEmbeddedSettingsService()
	})

	if err := app.ensureSettingsServiceStarted(); err != nil {
		t.Fatalf("ensureSettingsServiceStarted() error = %v", err)
	}

	endpoints := []string{"/", "/api/config", "/api/auth/status", "/api/app/session"}
	for _, path := range endpoints {
		resp, err := http.Get(app.settingsURL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("GET %s status = %d, want %d", path, resp.StatusCode, http.StatusOK)
		}
		_ = resp.Body.Close()
	}
}

type stubEmbeddedGatewayService struct {
	startFn func() error
	stopFn  func(context.Context) error
}

func (s *stubEmbeddedGatewayService) Start(ctx context.Context) error {
	if s.startFn != nil {
		return s.startFn()
	}
	return nil
}

func (s *stubEmbeddedGatewayService) Stop(ctx context.Context) error {
	if s.stopFn != nil {
		return s.stopFn(ctx)
	}
	return nil
}

func TestEnsureGatewayServiceStartedRunsEmbeddedGatewayWhenPortIsFree(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	var server *http.Server
	startCalls := 0
	stopCalls := 0
	app := &App{
		gatewayURL: "http://" + addr,
		gatewayServiceFactory: func() (gatewayServiceController, error) {
			return &stubEmbeddedGatewayService{
				startFn: func() error {
					startCalls++
					mux := http.NewServeMux()
					mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"status":"ok"}`))
					})
					mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{"status":"ok"}`))
					})
					var err error
					server = &http.Server{Handler: mux}
					serverListener, err := net.Listen("tcp", addr)
					if err != nil {
						return err
					}
					go server.Serve(serverListener)
					return nil
				},
				stopFn: func(ctx context.Context) error {
					stopCalls++
					if server != nil {
						return server.Shutdown(ctx)
					}
					return nil
				},
			}, nil
		},
	}
	t.Cleanup(func() {
		app.stopEmbeddedGatewayService()
	})

	if err := app.ensureGatewayServiceStarted(); err != nil {
		t.Fatalf("ensureGatewayServiceStarted() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("gateway startCalls = %d, want 1", startCalls)
	}

	resp, err := http.Get(app.gatewayURL + "/ready")
	if err != nil {
		t.Fatalf("GET gateway /ready error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET gateway /ready status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	app.stopEmbeddedGatewayService()
	if stopCalls != 1 {
		t.Fatalf("gateway stopCalls = %d, want 1", stopCalls)
	}
}

func TestEmbeddedGatewayServiceRelaysLoggerOutputIntoGatewayLogs(t *testing.T) {
	app := &App{
		gatewayServiceFactory: func() (gatewayServiceController, error) {
			return &stubEmbeddedGatewayService{}, nil
		},
	}
	t.Cleanup(func() {
		app.stopEmbeddedGatewayService()
	})

	if err := app.startEmbeddedGatewayService(); err != nil {
		t.Fatalf("startEmbeddedGatewayService() error = %v", err)
	}

	pinchlogger.InfoC("gateway-test", "embedded observer line")
	deadline := time.Now().Add(2 * time.Second)
	for {
		app.gatewayLogMu.Lock()
		lines := append([]string(nil), app.gatewayLogLines...)
		app.gatewayLogMu.Unlock()
		found := false
		for _, line := range lines {
			if strings.Contains(line, "embedded observer line") {
				found = true
				break
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gateway logs = %#v, want embedded observer line", lines)
		}
		time.Sleep(10 * time.Millisecond)
	}

	app.stopEmbeddedGatewayService()
	app.gatewayLogMu.Lock()
	app.gatewayLogLines = nil
	app.gatewayLogMu.Unlock()
	pinchlogger.Info("after embedded gateway stop")
	time.Sleep(50 * time.Millisecond)

	app.gatewayLogMu.Lock()
	defer app.gatewayLogMu.Unlock()
	if len(app.gatewayLogLines) != 0 {
		t.Fatalf("gatewayLogLines after stop = %#v, want empty", app.gatewayLogLines)
	}
}

func TestEnsureSettingsServiceMatchesHomeRejectsMismatchedHome(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", homeDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(settingsConfigResponse{
			Path: filepath.Join(t.TempDir(), "foreign-home", "config.json"),
			Home: filepath.Join(t.TempDir(), "foreign-home"),
		})
	}))
	defer server.Close()

	err := ensureSettingsServiceMatchesHome(server.URL)
	if err == nil {
		t.Fatal("ensureSettingsServiceMatchesHome() expected mismatch error")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"settings service uses PINCHBOT_HOME", homeDir}) {
		t.Fatalf("ensureSettingsServiceMatchesHome() error = %q, want mismatch details", got)
	}
}

func TestEnsureSettingsServiceMatchesHomeRejectsMissingRuntimeHome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"path": "C:/tmp/config.json"})
	}))
	defer server.Close()

	err := ensureSettingsServiceMatchesHome(server.URL)
	if err == nil {
		t.Fatal("ensureSettingsServiceMatchesHome() expected missing-home error")
	}
	if got := err.Error(); got != "settings service did not report its PINCHBOT_HOME" {
		t.Fatalf("ensureSettingsServiceMatchesHome() error = %q, want missing-home error", got)
	}
}

func TestEnsureSettingsServiceMatchesHomeAcceptsLegacyConfigPathFallback(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", homeDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(settingsConfigResponse{
			Path: filepath.Join(homeDir, "config.json"),
		})
	}))
	defer server.Close()

	if err := ensureSettingsServiceMatchesHome(server.URL); err != nil {
		t.Fatalf("ensureSettingsServiceMatchesHome() legacy fallback error = %v", err)
	}
}

func TestInferLegacySettingsServiceHomeRejectsNonDefaultConfigPath(t *testing.T) {
	got := inferLegacySettingsServiceHome(filepath.Join("C:\\tmp", "nested", "custom.json"))
	if got != "" {
		t.Fatalf("inferLegacySettingsServiceHome() = %q, want empty", got)
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
