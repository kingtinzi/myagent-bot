package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

func TestAppLoginReturnsLocalizedInvalidCredentialsWithoutProtocolPrefix(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "邮箱或密码错误", http.StatusBadRequest)
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", strings.NewReader(`{"email":"user@example.com","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "邮箱或密码错误") {
		t.Fatalf("body = %q, want localized invalid-credentials guidance", body)
	}
	if strings.Contains(body, "platform api returned") {
		t.Fatalf("body = %q, should not leak protocol prefix", body)
	}
}

func TestAppSignupRejectsInvalidEmailBeforePlatformRequest(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	called := false
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected upstream call for malformed email: %s", r.URL.Path)
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/signup", strings.NewReader(`{"email":"bad-email","password":"secret","username":"阿星"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if called {
		t.Fatal("expected malformed email to be rejected before calling platform service")
	}
	if !strings.Contains(rec.Body.String(), platformapi.InvalidEmailFormatMessage) {
		t.Fatalf("body = %q, want localized invalid-email-format guidance", rec.Body.String())
	}
}

func TestAppAuthEndpointRejectsMissingAccessToken(t *testing.T) {
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
			t.Fatalf("path = %q, want /auth/login", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(platformapi.AuthResponse{
			Session: platformapi.Session{
				UserID:    "user-1",
				Email:     "user@example.com",
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
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

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "未返回有效会话") {
		t.Fatalf("body = %q, want missing-session-token guidance", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected invalid login session not to be persisted, err = %v", err)
	}
}

func TestAppSessionMasksWalletSyncError(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := platformapi.NewFileSessionStore(dir).Save(platformapi.Session{
		AccessToken: "token-1",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wallet" {
			t.Fatalf("path = %q, want /wallet", r.URL.Path)
		}
		http.Error(w, "dial tcp 10.0.0.1:443: connect: connection refused", http.StatusBadGateway)
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
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	walletErr, _ := resp["wallet_error"].(string)
	if !strings.Contains(walletErr, "平台服务暂不可用") {
		t.Fatalf("wallet_error = %q, want sanitized wallet sync warning", walletErr)
	}
	if strings.Contains(walletErr, "10.0.0.1") || strings.Contains(strings.ToLower(walletErr), "connection refused") {
		t.Fatalf("wallet_error = %q, should not leak raw transport details", walletErr)
	}
}

func TestAppAuthAgreementsEndpointReturnsSignupDocuments(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agreements/current" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/agreements/current")
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty for public agreements", got)
		}
		_ = json.NewEncoder(w).Encode([]platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议"},
			{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
			{Key: "recharge_service", Version: "v1", Title: "充值协议"},
		})
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/app/auth/agreements", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var docs []platformapi.AgreementDocument
	if err := json.NewDecoder(rec.Body).Decode(&docs); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %#v, want signup agreements only", docs)
	}
}

func TestAppSignupForwardsAgreementsToPlatform(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	var gotSignup platformapi.AuthRequest
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/signup":
			if err := json.NewDecoder(r.Body).Decode(&gotSignup); err != nil {
				t.Fatalf("decode signup request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(platformapi.AuthResponse{
				Session: platformapi.Session{
					AccessToken: "token-1",
					UserID:      "user-1",
					Email:       "user@example.com",
					ExpiresAt:   time.Now().Add(time.Hour).Unix(),
				},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/signup", strings.NewReader(`{"email":"user@example.com","password":"secret","username":"阿星","agreements":[{"key":"user_terms","version":"v1","title":"用户协议"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(gotSignup.Agreements) != 1 || gotSignup.Agreements[0].Key != "user_terms" {
		t.Fatalf("agreements = %#v, want forwarded signup agreements", gotSignup.Agreements)
	}
	if gotSignup.Username != "阿星" {
		t.Fatalf("signup request = %#v, want forwarded username", gotSignup)
	}
}

func TestAppSignupDoesNotPersistSessionWhenPlatformRejects(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/signup":
			http.Error(w, "agreement privacy_policy version v1 must be accepted before signup", http.StatusBadRequest)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/signup", strings.NewReader(`{"email":"user@example.com","password":"secret","agreements":[{"key":"user_terms","version":"v1","title":"用户协议"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected rejected signup not to persist session, err = %v", err)
	}
}

func TestAppSignupRetriesAgreementAcceptanceWhenPlatformRequestsRecovery(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	var (
		acceptCalls int
		gotAccept   platformapi.AcceptAgreementsRequest
	)
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/signup":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session": map[string]any{
					"access_token": "token-1",
					"user_id":      "user-1",
					"email":        "user@example.com",
					"expires_at":   time.Now().Add(time.Hour).Unix(),
				},
				"agreement_sync_required": true,
				"warning":                 "注册已成功，但协议确认同步失败，请在充值前重新确认协议",
			})
		case "/agreements/accept":
			acceptCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
			}
			if err := json.NewDecoder(r.Body).Decode(&gotAccept); err != nil {
				t.Fatalf("decode accept request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/signup", strings.NewReader(`{"email":"user@example.com","password":"secret","agreements":[{"key":"user_terms","version":"v1","title":"用户协议"},{"key":"privacy_policy","version":"v1","title":"隐私政策"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if acceptCalls != 1 {
		t.Fatalf("acceptCalls = %d, want 1 recovery call", acceptCalls)
	}
	if len(gotAccept.Agreements) != 2 {
		t.Fatalf("agreements = %#v, want forwarded signup agreements for recovery", gotAccept.Agreements)
	}
}

func TestAppSessionRetainsPendingAgreementRecoveryState(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:1"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/signup":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session": map[string]any{
					"access_token": "token-1",
					"user_id":      "user-1",
					"email":        "user@example.com",
					"expires_at":   time.Now().Add(time.Hour).Unix(),
				},
				"agreement_sync_required": true,
				"warning":                 "注册已成功，但协议确认同步失败，请在充值前重新确认协议",
			})
		case "/agreements/accept":
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
		case "/wallet":
			_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 66, Currency: "CNY"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer platformServer.Close()

	cfg.PlatformAPI.BaseURL = platformServer.URL
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() update error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterAppPlatformAPI(mux, configPath)

	signupReq := httptest.NewRequest(http.MethodPost, "/api/app/auth/signup", strings.NewReader(`{"email":"user@example.com","password":"secret","agreements":[{"key":"user_terms","version":"v1","title":"用户协议"},{"key":"privacy_policy","version":"v1","title":"隐私政策"}]}`))
	signupReq.Header.Set("Content-Type", "application/json")
	signupRec := httptest.NewRecorder()
	mux.ServeHTTP(signupRec, signupReq)
	if signupRec.Code != http.StatusOK {
		t.Fatalf("signup status = %d, want %d: %s", signupRec.Code, http.StatusOK, signupRec.Body.String())
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/app/session", nil)
	sessionRec := httptest.NewRecorder()
	mux.ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d: %s", sessionRec.Code, http.StatusOK, sessionRec.Body.String())
	}
	var resp struct {
		Authenticated bool                      `json:"authenticated"`
		Session       platformapi.SessionView   `json:"session"`
		Wallet        platformapi.WalletSummary `json:"wallet"`
	}
	if err := json.NewDecoder(sessionRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if !resp.Authenticated || !resp.Session.AgreementSyncPending {
		t.Fatalf("resp = %#v, want pending agreement recovery state", resp)
	}
	if resp.Session.Warning != "注册已成功，但协议确认同步失败，请在充值前重新确认协议" {
		t.Fatalf("warning = %q, want persisted recovery warning", resp.Session.Warning)
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

func TestAppBackendStatusEndpointReturnsUnifiedShape(t *testing.T) {
	dir := t.TempDir()
	bindSessionHome(t, dir)
	configPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()

	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("gateway path = %q, want %q", r.URL.Path, "/health")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gatewayServer.Close()
	gatewayParsed, err := url.Parse(gatewayServer.URL)
	if err != nil {
		t.Fatalf("Parse gateway URL: %v", err)
	}
	gatewayHost, gatewayPortRaw, err := net.SplitHostPort(gatewayParsed.Host)
	if err != nil {
		t.Fatalf("Split gateway host/port: %v", err)
	}
	gatewayPort, err := strconv.Atoi(gatewayPortRaw)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", gatewayPortRaw, err)
	}
	cfg.Gateway.Host = gatewayHost
	cfg.Gateway.Port = gatewayPort

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("platform path = %q, want %q", r.URL.Path, "/health")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer platformServer.Close()
	cfg.PlatformAPI.BaseURL = platformServer.URL

	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	RegisterAppPlatformAPI(mux, configPath)
	settingsServer := httptest.NewServer(mux)
	defer settingsServer.Close()

	resp, err := http.Get(settingsServer.URL + "/api/app/backend-status")
	if err != nil {
		t.Fatalf("GET backend status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var status platformapi.BackendStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if status.GatewayURL != gatewayServer.URL || !status.GatewayHealthy {
		t.Fatalf("gateway status = %#v, want healthy gateway summary", status)
	}
	if status.PlatformURL != platformServer.URL || !status.PlatformHealthy {
		t.Fatalf("platform status = %#v, want healthy platform summary", status)
	}
	if status.SettingsURL != settingsServer.URL || !status.SettingsHealthy {
		t.Fatalf("settings status = %#v, want derived settings service summary", status)
	}
}
