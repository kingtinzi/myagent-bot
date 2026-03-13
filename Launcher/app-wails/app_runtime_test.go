package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
