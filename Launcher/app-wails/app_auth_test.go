package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pconfig "github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func TestGetAuthStateClearsUnauthorizedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer server.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   store,
	}

	state := app.GetAuthState()
	if state.Authenticated {
		t.Fatal("expected unauthorized session to be treated as signed out")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestChatClearsUnauthorizedSession(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer gateway.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		ctx:          context.Background(),
		gatewayURL:   gateway.URL,
		sessionStore: store,
	}

	_, err := app.Chat("hello", nil)
	if err == nil {
		t.Fatal("expected unauthorized chat session to fail")
	}
	if got := err.Error(); got != authRequiredErrorPrefix+"登录状态已过期，请重新登录" {
		t.Fatalf("Chat() error = %q, want auth-required prefix", got)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestGetAuthStateClearsLocallyExpiredSessionWithoutUpstreamCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"user_id":"user-1","balance_fen":0,"currency":"CNY","updated_unix":1}`))
	}))
	defer server.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   store,
	}

	state := app.GetAuthState()
	if state.Authenticated {
		t.Fatal("expected expired local session to be treated as signed out")
	}
	if called {
		t.Fatal("expected expired session to be rejected locally before upstream wallet call")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestChatClearsLocallyExpiredSessionWithoutGatewayCall(t *testing.T) {
	called := false
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":"ok"}`))
	}))
	defer gateway.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		ctx:          context.Background(),
		gatewayURL:   gateway.URL,
		sessionStore: store,
	}

	_, err := app.Chat("hello", nil)
	if err == nil {
		t.Fatal("expected locally expired chat session to fail")
	}
	if got := err.Error(); got != authRequiredErrorPrefix+"登录状态已过期，请重新登录" {
		t.Fatalf("Chat() error = %q, want auth-required expired-session error", got)
	}
	if called {
		t.Fatal("expected expired session to be rejected locally before gateway call")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestListAuthAgreementsReturnsSignupDocuments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agreements/current" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/agreements/current")
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty for public agreements", got)
		}
		_ = json.NewEncoder(w).Encode([]platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议"},
			{Key: "privacy_policy", Version: "v2", Title: "隐私政策"},
			{Key: "recharge_service", Version: "v3", Title: "充值协议"},
		})
	}))
	defer server.Close()

	app := &App{platformClient: platformapi.NewClient(server.URL)}
	docs, err := app.ListAuthAgreements()
	if err != nil {
		t.Fatalf("ListAuthAgreements() error = %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %#v, want signup agreements only", docs)
	}
	if docs[0].Key != "privacy_policy" && docs[0].Key != "user_terms" {
		t.Fatalf("docs = %#v, want signup agreement keys", docs)
	}
}

func TestSignUpWithAgreementsForwardsDocumentsInSignupRequest(t *testing.T) {
	var gotSignup platformapi.AuthRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case "/wallet":
			_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 0, Currency: "CNY"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	baseDir := t.TempDir()
	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   platformapi.NewFileSessionStore(baseDir),
	}
	docs := []platformapi.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
	}
	state, err := app.SignUpWithAgreements("user@example.com", "secret", "阿星", docs)
	if err != nil {
		t.Fatalf("SignUpWithAgreements() error = %v", err)
	}
	if !state.Authenticated {
		t.Fatalf("state = %#v, want authenticated state", state)
	}
	if len(gotSignup.Agreements) != 2 {
		t.Fatalf("signup agreements = %#v, want two forwarded signup agreements", gotSignup.Agreements)
	}
	if gotSignup.Username != "阿星" {
		t.Fatalf("signup request = %#v, want username forwarded", gotSignup)
	}
}

func TestSignUpWithAgreementsDoesNotPersistSessionWhenBackendRejects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/signup":
			http.Error(w, "agreement user_terms version v1 must be accepted before signup", http.StatusBadRequest)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	baseDir := t.TempDir()
	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   platformapi.NewFileSessionStore(baseDir),
	}
	_, err := app.SignUpWithAgreements("user@example.com", "secret", "阿星", []platformapi.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议"},
	})
	if err == nil || !strings.Contains(err.Error(), "must be accepted before signup") {
		t.Fatalf("error = %v, want backend signup agreement validation failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected session file to be removed, err = %v", statErr)
	}
}

func TestSignUpWithAgreementsRetriesAgreementAcceptanceWhenSignupNeedsRecovery(t *testing.T) {
	var (
		acceptCalls int
		gotAccept   platformapi.AcceptAgreementsRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				"warning":                 "signup completed, but agreement acceptance must be retried",
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
		case "/wallet":
			_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 0, Currency: "CNY"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	baseDir := t.TempDir()
	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   platformapi.NewFileSessionStore(baseDir),
	}
	docs := []platformapi.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
	}
	state, err := app.SignUpWithAgreements("user@example.com", "secret", "阿星", docs)
	if err != nil {
		t.Fatalf("SignUpWithAgreements() error = %v", err)
	}
	if !state.Authenticated {
		t.Fatalf("state = %#v, want authenticated state", state)
	}
	if acceptCalls != 1 {
		t.Fatalf("acceptCalls = %d, want 1 recovery call", acceptCalls)
	}
	if len(gotAccept.Agreements) != 2 {
		t.Fatalf("accept agreements = %#v, want forwarded agreements for recovery", gotAccept.Agreements)
	}
}

func TestSignUpWithAgreementsPersistsPendingAgreementRecoveryState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_ = json.NewEncoder(w).Encode(platformapi.WalletSummary{UserID: "user-1", BalanceFen: 0, Currency: "CNY"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	baseDir := t.TempDir()
	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   platformapi.NewFileSessionStore(baseDir),
	}
	docs := []platformapi.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
	}
	state, err := app.SignUpWithAgreements("user@example.com", "secret", "阿星", docs)
	if err != nil {
		t.Fatalf("SignUpWithAgreements() error = %v", err)
	}
	if !state.Authenticated || !state.AgreementSyncPending {
		t.Fatalf("state = %#v, want authenticated pending-recovery state", state)
	}
	if state.Warning != "注册已成功，但协议确认同步失败，请在充值前重新确认协议" {
		t.Fatalf("warning = %q, want persisted recovery warning", state.Warning)
	}

	reloaded := app.GetAuthState()
	if !reloaded.Authenticated || !reloaded.AgreementSyncPending {
		t.Fatalf("reloaded state = %#v, want pending recovery to survive refresh", reloaded)
	}
	if reloaded.Warning != state.Warning {
		t.Fatalf("reloaded warning = %q, want %q", reloaded.Warning, state.Warning)
	}
}

func TestGetOfficialAccessStateReturnsPlatformState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/official/access" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/official/access")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		_ = json.NewEncoder(w).Encode(platformapi.OfficialAccessState{
			Enabled:          true,
			BalanceFen:       88,
			Currency:         "CNY",
			LowBalance:       true,
			ModelsConfigured: 2,
		})
	}))
	defer server.Close()

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

	app := &App{platformClient: platformapi.NewClient(server.URL), sessionStore: store}
	state, err := app.GetOfficialAccessState()
	if err != nil {
		t.Fatalf("GetOfficialAccessState() error = %v", err)
	}
	if !state.Enabled || !state.LowBalance || state.ModelsConfigured != 2 {
		t.Fatalf("state = %#v, want returned official access summary", state)
	}
}

func TestListOfficialModelsReturnsCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/official/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/official/models")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		_ = json.NewEncoder(w).Encode([]platformapi.OfficialModel{
			{ID: "official-basic", Name: "Official Basic", PricingVersion: "v1"},
			{ID: "official-pro", Name: "Official Pro", PricingVersion: "v2"},
		})
	}))
	defer server.Close()

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

	app := &App{platformClient: platformapi.NewClient(server.URL), sessionStore: store}
	models, err := app.ListOfficialModels()
	if err != nil {
		t.Fatalf("ListOfficialModels() error = %v", err)
	}
	if len(models) != 2 || models[0].ID != "official-basic" {
		t.Fatalf("models = %#v, want returned official catalog", models)
	}
}

func TestGetOfficialPanelSnapshotReturnsAccessAndModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		switch r.URL.Path {
		case "/official/access":
			_ = json.NewEncoder(w).Encode(platformapi.OfficialAccessState{
				Enabled:          true,
				BalanceFen:       88,
				Currency:         "CNY",
				LowBalance:       true,
				ModelsConfigured: 2,
			})
		case "/official/models":
			_ = json.NewEncoder(w).Encode([]platformapi.OfficialModel{
				{ID: "official-basic", Name: "Official Basic", PricingVersion: "v1"},
				{ID: "official-pro", Name: "Official Pro", PricingVersion: "v2"},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

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

	app := &App{platformClient: platformapi.NewClient(server.URL), sessionStore: store}
	snapshot, err := app.GetOfficialPanelSnapshot()
	if err != nil {
		t.Fatalf("GetOfficialPanelSnapshot() error = %v", err)
	}
	if !snapshot.Access.Enabled || len(snapshot.Models) != 2 || snapshot.Models[0].ID != "official-basic" {
		t.Fatalf("snapshot = %#v, want combined access and model data", snapshot)
	}
}

func TestGetOfficialPanelSnapshotClearsUnauthorizedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer server.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{platformClient: platformapi.NewClient(server.URL), sessionStore: store}
	_, err := app.GetOfficialPanelSnapshot()
	if err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("error = %v, want session expired after unauthorized official panel fetch", err)
	}
	if _, statErr := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected session file to be removed, err = %v", statErr)
	}
}

func TestGetBackendStatusReturnsStructuredSummary(t *testing.T) {
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("gateway path = %q, want %q", r.URL.Path, "/health")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gatewayServer.Close()

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("platform path = %q, want %q", r.URL.Path, "/health")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer platformServer.Close()

	settingsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			t.Fatalf("settings path = %q, want %q", r.URL.Path, "/api/config")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer settingsServer.Close()

	app := &App{
		gatewayURL:  gatewayServer.URL,
		platformURL: platformServer.URL,
		settingsURL: settingsServer.URL,
	}

	status := app.GetBackendStatus()
	if status.GatewayURL != gatewayServer.URL || !status.GatewayHealthy {
		t.Fatalf("gateway status = %#v, want healthy gateway summary", status)
	}
	if status.PlatformURL != platformServer.URL || !status.PlatformHealthy {
		t.Fatalf("platform status = %#v, want healthy platform summary", status)
	}
	if status.SettingsURL != settingsServer.URL || !status.SettingsHealthy {
		t.Fatalf("settings status = %#v, want healthy settings summary", status)
	}
}

func TestNewAppUsesConfiguredPlatformURLWhenExplicitURLEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv(pconfig.PinchBotHomeEnv, home)
	t.Setenv(pconfig.PinchBotConfigEnv, "")

	cfg := pconfig.DefaultConfig()
	cfg.PlatformAPI.BaseURL = "http://127.0.0.1:28793"
	if err := pconfig.SaveConfig(pconfig.GetConfigPath(), cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	app := NewApp("http://127.0.0.1:18800", "http://127.0.0.1:18790", "")
	if app.platformURL != "http://127.0.0.1:28793" {
		t.Fatalf("platformURL = %q, want configured base URL", app.platformURL)
	}
	if got := app.platformClient.BaseURL(); got != "http://127.0.0.1:28793" {
		t.Fatalf("platformClient.BaseURL() = %q, want configured base URL", got)
	}
}

func TestListAuthAgreementsUsesConfiguredPlatformURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agreements/current" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/agreements/current")
		}
		_ = json.NewEncoder(w).Encode([]platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议"},
			{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
		})
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv(pconfig.PinchBotHomeEnv, home)
	t.Setenv(pconfig.PinchBotConfigEnv, "")

	cfg := pconfig.DefaultConfig()
	cfg.PlatformAPI.BaseURL = server.URL
	if err := pconfig.SaveConfig(pconfig.GetConfigPath(), cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	app := NewApp("http://127.0.0.1:18800", "http://127.0.0.1:18790", "")
	docs, err := app.ListAuthAgreements()
	if err != nil {
		t.Fatalf("ListAuthAgreements() error = %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %#v, want 2 configured-platform agreements", docs)
	}
}
