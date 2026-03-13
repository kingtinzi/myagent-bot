package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func bindSessionHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PINCHBOT_HOME", dir)
	t.Setenv("PICOCLAW_HOME", "")
}

func TestSyncOfficialModelsIntoConfigAddsAndRemovesModels(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:18791"
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "gpt-5.2", Model: "openai/gpt-5.2"},
		{ModelName: "official-legacy", Model: "official/legacy", APIBase: "http://old-platform"},
	}
	cfg.Agents.Defaults.ModelName = "official-legacy"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	result, err := syncOfficialModelsIntoConfig(configPath, []platformapi.OfficialModel{
		{ID: "basic", Name: "Basic", Enabled: true},
		{ID: "legacy", Name: "Legacy", Enabled: false},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}
	if result.Added != 1 || result.Removed != 1 {
		t.Fatalf("result = %#v, want added=1 removed=1", result)
	}
	if !result.DefaultChanged || result.DefaultModel != "official-basic" {
		t.Fatalf("default change = %#v, want changed official-basic", result)
	}

	saved, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if saved.Agents.Defaults.GetModelName() != "official-basic" {
		t.Fatalf("default model = %q, want %q", saved.Agents.Defaults.GetModelName(), "official-basic")
	}
	if len(saved.ModelList) != 2 {
		t.Fatalf("len(model_list) = %d, want 2", len(saved.ModelList))
	}
	if saved.ModelList[1].Model != "official/basic" {
		t.Fatalf("official model = %q, want %q", saved.ModelList[1].Model, "official/basic")
	}
	if saved.ModelList[1].APIBase != "http://127.0.0.1:18791" {
		t.Fatalf("official api_base = %q, want %q", saved.ModelList[1].APIBase, "http://127.0.0.1:18791")
	}
}

func TestAppModelsSyncEndpointPreservesCustomAlias(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	cfg.ModelList = []config.ModelConfig{
		{ModelName: "my-official-model", Model: "official/pro", APIBase: "http://old-platform"},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken: "token-1",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		if r.URL.Path != "/official/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/official/models")
		}
		_ = json.NewEncoder(w).Encode([]platformapi.OfficialModel{
			{ID: "pro", Name: "Pro", Enabled: true},
		})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/models/sync", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result officialModelSyncResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode sync result: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Updated)
	}
	if !result.DefaultChanged || result.DefaultModel != "my-official-model" {
		t.Fatalf("default change = %#v, want changed my-official-model", result)
	}

	saved, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(saved.ModelList) != 1 {
		t.Fatalf("len(model_list) = %d, want 1", len(saved.ModelList))
	}
	if saved.ModelList[0].ModelName != "my-official-model" {
		t.Fatalf("model_name = %q, want %q", saved.ModelList[0].ModelName, "my-official-model")
	}
	if saved.ModelList[0].APIBase != platformServer.URL {
		t.Fatalf("api_base = %q, want %q", saved.ModelList[0].APIBase, platformServer.URL)
	}
}

func TestPlatformContextRejectsMissingSession(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/app/models", nil)
	rec := httptest.NewRecorder()
	client, session, ok := platformContext(rec, req, configPath)
	if ok || client != nil || session.AccessToken != "" {
		t.Fatal("expected missing session to fail platformContext")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAppSessionEndpointClearsUnauthorizedSession(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/app/session", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, _ := resp["authenticated"].(bool); got {
		t.Fatal("expected unauthorized upstream session to be cleared")
	}
	if _, err := os.Stat(filepath.Join(dir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be deleted, err = %v", err)
	}
}

func TestAppSessionEndpointDoesNotExposeTokens(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken:  "token-1",
		RefreshToken: "refresh-1",
		UserID:       "user-1",
		Email:        "user@example.com",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wallet" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/wallet")
		}
		_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 99, Currency: "CNY"})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/app/session", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "access_token") || strings.Contains(body, "refresh_token") {
		t.Fatalf("response leaked tokens: %s", body)
	}
	var resp struct {
		Authenticated bool                    `json:"authenticated"`
		Session       platformapi.SessionView `json:"session"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("expected authenticated session response")
	}
	if resp.Session.UserID != "user-1" || resp.Session.Email != "user@example.com" {
		t.Fatalf("session = %#v, want user fields only", resp.Session)
	}
}

func TestAppAuthEndpointStoresFullSessionButReturnsPublicView(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/auth/login")
		}
		_ = json.NewEncoder(w).Encode(platformapi.AuthResponse{
			Session: platformapi.Session{
				AccessToken:  "token-1",
				RefreshToken: "refresh-1",
				UserID:       "user-1",
				Email:        "user@example.com",
				ExpiresAt:    time.Now().Add(time.Hour).Unix(),
			},
		})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", strings.NewReader(`{"email":"user@example.com","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "access_token") || strings.Contains(body, "refresh_token") {
		t.Fatalf("response leaked tokens: %s", body)
	}
	var resp struct {
		Session platformapi.SessionView `json:"session"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Session.UserID != "user-1" || resp.Session.Email != "user@example.com" {
		t.Fatalf("session = %#v, want public session view", resp.Session)
	}
	stored, err := platformapi.NewFileSessionStore(dir).Load()
	if err != nil {
		t.Fatalf("Load stored session: %v", err)
	}
	if stored.AccessToken != "token-1" || stored.RefreshToken != "refresh-1" {
		t.Fatalf("stored session = %#v, want full tokens persisted", stored)
	}
}

func TestAppSessionEndpointClearsExpiredSessionWithoutUpstreamCall(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	called := false
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 99, Currency: "CNY"})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/app/session", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, _ := resp["authenticated"].(bool); got {
		t.Fatal("expected expired local session to be cleared")
	}
	if called {
		t.Fatal("expected expired local session to skip upstream wallet call")
	}
	if _, err := os.Stat(filepath.Join(dir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be deleted, err = %v", err)
	}
}

func TestAppWalletEndpointRejectsExpiredSessionWithoutUpstreamCall(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	called := false
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 99, Currency: "CNY"})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/app/wallet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("expected expired local session to skip upstream wallet request")
	}
	if _, err := os.Stat(filepath.Join(dir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be deleted, err = %v", err)
	}
}
