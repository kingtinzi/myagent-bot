package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"
)

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
