package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"

	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/runtimeconfig"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/upstream"
)

func TestServerRejectsMissingBearerToken(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServerReturnsWalletForAuthenticatedUser(t *testing.T) {
	store := service.NewMemoryStore()
	store.SetBalance("user-1", 1234)
	svc := service.NewService(store, nil)
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var wallet service.WalletSummary
	if err := json.NewDecoder(rec.Body).Decode(&wallet); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if wallet.BalanceFen != 1234 {
		t.Fatalf("balance = %d, want 1234", wallet.BalanceFen)
	}
}

func TestServerReturnsOfficialAccessState(t *testing.T) {
	store := service.NewMemoryStore()
	store.SetBalance("user-1", 66)
	svc := service.NewService(store, nil)
	svc.SetOfficialModels([]service.OfficialModel{{ID: "official-basic", Name: "Official Basic", Enabled: true}})
	svc.SetPricingCatalog([]service.PricingRule{{ModelID: "official-basic", Version: "v20260313", FallbackPriceFen: 10}})
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/official/access", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var state service.OfficialAccessState
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !state.Enabled || !state.LowBalance {
		t.Fatalf("state = %#v, want enabled low-balance official access", state)
	}
	if state.MinimumReserveFen != 10 {
		t.Fatalf("minimum_reserve_fen = %d, want 10", state.MinimumReserveFen)
	}
	if state.MinimumRechargeAmountFen != 10 {
		t.Fatalf("minimum_recharge_amount_fen = %d, want 10", state.MinimumRechargeAmountFen)
	}
}

func TestServerKeepsLegacyAdminUIAtAdminAndServesNewSPAAtAdminV2(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, nil, nil)

	legacyReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	legacyRec := httptest.NewRecorder()
	server.ServeHTTP(legacyRec, legacyReq)

	if legacyRec.Code != http.StatusOK {
		t.Fatalf("legacy /admin status = %d, want %d", legacyRec.Code, http.StatusOK)
	}
	if !strings.Contains(legacyRec.Body.String(), "响应式单文件管理界面") {
		t.Fatalf("legacy /admin body = %q, want existing single-file admin marker", legacyRec.Body.String())
	}

	v2Req := httptest.NewRequest(http.MethodGet, "/admin-v2", nil)
	v2Rec := httptest.NewRecorder()
	server.ServeHTTP(v2Rec, v2Req)

	if v2Rec.Code != http.StatusOK {
		t.Fatalf("/admin-v2 status = %d, want %d", v2Rec.Code, http.StatusOK)
	}
	if !strings.Contains(v2Rec.Body.String(), `id="root"`) {
		t.Fatalf("/admin-v2 body = %q, want SPA root container", v2Rec.Body.String())
	}
	if strings.Contains(v2Rec.Body.String(), "响应式单文件管理界面") {
		t.Fatalf("/admin-v2 body should not serve legacy single-file admin")
	}
}

func TestServerServesAdminV2DeepLinksWithSPAIndex(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin-v2/dashboard", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/admin-v2/dashboard status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `id="root"`) {
		t.Fatalf("/admin-v2/dashboard body = %q, want SPA index fallback", rec.Body.String())
	}
}

func TestServerCreatesRechargeOrder(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	body, _ := json.Marshal(service.CreateOrderInput{AmountFen: 8800, Channel: "manual"})
	req := httptest.NewRequest(http.MethodPost, "/wallet/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestServerRejectsRechargeOrderBelowMinimum(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	body, _ := json.Marshal(service.CreateOrderInput{AmountFen: 9, Channel: "manual"})
	req := httptest.NewRequest(http.MethodPost, "/wallet/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServerReturnsWalletOrder(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 8800, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet/orders/"+order.ID, nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got service.RechargeOrder
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != order.ID || got.AmountFen != 8800 {
		t.Fatalf("order = %#v, want created order %q", got, order.ID)
	}
}

func TestServerReturnsOfficialModels(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetOfficialModels([]service.OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
		{ID: "official-hidden", Name: "Official Hidden", Enabled: false},
	})
	svc.SetPricingRules(map[string]service.PricingRule{
		"official-basic": {ModelID: "official-basic", FallbackPriceFen: 8},
	})
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/official/models", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var models []service.OfficialModel
	if err := json.NewDecoder(rec.Body).Decode(&models); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "official-basic" {
		t.Fatalf("models = %#v, want official-basic", models)
	}
}

func TestOfficialChatDoesNotLeakUpstreamErrorDetails(t *testing.T) {
	store := service.NewMemoryStore()
	store.SetBalance("user-1", 500)
	svc := service.NewService(store, nil)
	svc.SetOfficialModels([]service.OfficialModel{{ID: "official-basic", Name: "Official Basic", Enabled: true}})
	svc.SetPricingCatalog([]service.PricingRule{{ModelID: "official-basic", Version: "v1", FallbackPriceFen: 10}})
	svc.SetOfficialProxyClient(stubProxyClient{
		err: errors.New("dial https://secret.example.com/v1 failed: <html>bad gateway</html>"),
	})
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, nil, nil)

	body, _ := json.Marshal(platformapi.ChatProxyRequest{ModelID: "official-basic"})
	req := httptest.NewRequest(http.MethodPost, "/chat/official", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	responseBody := rec.Body.String()
	if strings.Contains(responseBody, "secret.example.com") || strings.Contains(strings.ToLower(responseBody), "<html>") {
		t.Fatalf("body = %q, want sanitized upstream failure", responseBody)
	}
}

func TestWriteAdminAuditLogsCSVNeutralizesFormulaInjection(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAdminAuditLogsCSV(rec, []service.AdminAuditLog{
		{
			CreatedUnix: 1,
			ActorUserID: "admin-1",
			ActorEmail:  "=cmd|' /C calc'!A0",
			Action:      "admin.audit",
			TargetType:  "wallet_account",
			TargetID:    "user-1",
			RiskLevel:   "high",
			Detail:      "@malicious",
		},
	})

	body := rec.Body.String()
	if strings.Contains(body, ",=cmd|' /C calc'!A0,") || strings.Contains(body, ",@malicious") {
		t.Fatalf("csv body = %q, want spreadsheet formula prefixes neutralized", body)
	}
}

func TestAdminRuntimeConfigRoundTrip(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "https://api.openai.com/v1",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, manager)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/runtime-config", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("runtime get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	revision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if revision == "" {
		t.Fatal("expected runtime config GET to return an ETag revision")
	}

	body, _ := json.Marshal(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "https://api.openai.com/v1",
				},
			},
		},
		OfficialModels: []service.OfficialModel{
			{ID: "official-basic", Name: "Official Basic", Enabled: true},
		},
		PricingRules: []service.PricingRule{
			{ModelID: "official-basic", FallbackPriceFen: 9},
		},
		Agreements: []service.AgreementDocument{
			{Key: "recharge_service", Version: "v1", Title: "Recharge Service Agreement"},
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/runtime-config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", revision)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodGet, "/official/models", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var models []service.OfficialModel
	if err := json.NewDecoder(rec.Body).Decode(&models); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "official-basic" {
		t.Fatalf("models = %#v, want official-basic", models)
	}

	logs, err := svc.ListAuditLogs(context.Background(), service.AuditLogFilter{Action: "admin.runtime_config.updated"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected runtime config update to be audited")
	}
}

func TestAdminRuntimeConfigReadRedactsSecretsAndPreservesPlaceholderOnSave(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com", "user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleSuperAdmin,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "https://api.openai.com/v1",
					APIKey:    "secret-key",
				},
			},
		},
		OfficialModels: []service.OfficialModel{
			{ID: "official-basic", Name: "Official Basic", Enabled: true},
		},
		PricingRules: []service.PricingRule{
			{ModelID: "official-basic", FallbackPriceFen: 8},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, manager)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/runtime-config", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("runtime get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var snapshot runtimeconfig.State
	if err := json.NewDecoder(getRec.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode runtime snapshot: %v", err)
	}
	if got := snapshot.OfficialRoutes[0].ModelConfig.APIKey; got != runtimeconfig.RedactedSecretPlaceholder {
		t.Fatalf("api_key = %q, want redacted placeholder", got)
	}
	revision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if revision == "" {
		t.Fatal("expected runtime config GET to return an ETag revision")
	}

	snapshot.OfficialRoutes[0].ModelConfig.Model = "openai/gpt-4.1-mini"
	body, _ := json.Marshal(snapshot)
	putReq := httptest.NewRequest(http.MethodPut, "/admin/runtime-config", bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(putReq)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("If-Match", revision)
	putRec := httptest.NewRecorder()
	server.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("runtime put status = %d, want %d: %s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	raw := manager.Snapshot()
	if got := raw.OfficialRoutes[0].ModelConfig.APIKey; got != "secret-key" {
		t.Fatalf("stored api_key = %q, want original secret preserved", got)
	}
	if got := raw.OfficialRoutes[0].ModelConfig.Model; got != "openai/gpt-4.1-mini" {
		t.Fatalf("stored model = %q, want updated model", got)
	}
}

func TestAdminRuntimeConfigGetReturnsRevisionAndRejectsStaleIfMatch(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com", "user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleSuperAdmin,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, manager)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/runtime-config", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("runtime get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	revision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if revision == "" {
		t.Fatal("expected runtime config GET to return an ETag revision")
	}

	if err := manager.Save(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4.1-mini",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	body, _ := json.Marshal(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4.1",
				},
			},
		},
	})
	putReq := httptest.NewRequest(http.MethodPut, "/admin/runtime-config", bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(putReq)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("If-Match", revision)
	putRec := httptest.NewRecorder()
	server.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("runtime put status = %d, want %d: %s", putRec.Code, http.StatusPreconditionFailed, putRec.Body.String())
	}
	if !strings.Contains(putRec.Body.String(), "配置已被其他管理员更新") {
		t.Fatalf("body = %q, want stale revision guidance", putRec.Body.String())
	}
}

func TestAdminRuntimeConfigRevisionChangesWhenSecretChanges(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleOperations,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-a",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, manager)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/runtime-config", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("runtime get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	initialRevision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if initialRevision == "" {
		t.Fatal("expected runtime config GET to return an ETag revision")
	}

	if err := manager.Save(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-b",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	getReq = httptest.NewRequest(http.MethodGet, "/admin/runtime-config", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec = httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("runtime get status after secret rotation = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	rotatedRevision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if rotatedRevision == "" {
		t.Fatal("expected runtime config GET after secret rotation to return an ETag revision")
	}
	if rotatedRevision == initialRevision {
		t.Fatalf("runtime config revision = %q, want change after secret rotation", rotatedRevision)
	}
}

func TestAdminRuntimeConfigPutRequiresRevision(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleOperations,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, manager)

	body, _ := json.Marshal(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4.1-mini",
				},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/admin/runtime-config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("runtime put status = %d, want %d: %s", rec.Code, http.StatusPreconditionRequired, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "保存前缺少配置版本") {
		t.Fatalf("body = %q, want missing revision guidance", rec.Body.String())
	}
}

func TestAdminSystemNoticesGetReturnsRevisionAndRejectsStaleIfMatch(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleSuperAdmin,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	if err := svc.SaveSystemNotices(context.Background(), []service.SystemNotice{
		{ID: "notice-1", Title: "Billing notice", Body: "initial", Severity: "info", Enabled: true},
	}); err != nil {
		t.Fatalf("SaveSystemNotices() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, nil)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/system-notices", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("system notices get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	revision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if revision == "" {
		t.Fatal("expected system notices GET to return an ETag revision")
	}

	if err := svc.SaveSystemNotices(context.Background(), []service.SystemNotice{
		{ID: "notice-1", Title: "Billing notice", Body: "changed", Severity: "warning", Enabled: true},
	}); err != nil {
		t.Fatalf("SaveSystemNotices() mutation error = %v", err)
	}

	body, _ := json.Marshal([]service.SystemNotice{
		{ID: "notice-2", Title: "Launch notice", Body: "new", Severity: "info", Enabled: true},
	})
	putReq := httptest.NewRequest(http.MethodPut, "/admin/system-notices", bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(putReq)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("If-Match", revision)
	putRec := httptest.NewRecorder()
	server.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("system notices put status = %d, want %d: %s", putRec.Code, http.StatusPreconditionFailed, putRec.Body.String())
	}
	if !strings.Contains(putRec.Body.String(), "配置已被其他管理员更新") {
		t.Fatalf("body = %q, want stale revision guidance", putRec.Body.String())
	}
}

func TestAdminModelRoutesReadRedactsSecrets(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleOperations,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: config.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-key",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, nil, manager)

	req := httptest.NewRequest(http.MethodGet, "/admin/model-routes", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var routes []upstream.OfficialRoute
	if err := json.NewDecoder(rec.Body).Decode(&routes); err != nil {
		t.Fatalf("decode routes: %v", err)
	}
	if len(routes) != 1 || routes[0].ModelConfig.APIKey != runtimeconfig.RedactedSecretPlaceholder {
		t.Fatalf("routes = %#v, want redacted route secrets", routes)
	}
}

func TestServerReturnsAgreements(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge Service Agreement",
		Content: "Recharge funds are used for official model calls.",
		URL:     "https://example.com/agreement",
	})
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/agreements/current", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var agreements []service.AgreementDocument
	if err := json.NewDecoder(rec.Body).Decode(&agreements); err != nil {
		t.Fatalf("decode agreements: %v", err)
	}
	if len(agreements) != 1 || agreements[0].Version != "v1" {
		t.Fatalf("agreements = %#v, want version v1", agreements)
	}
	if agreements[0].Content == "" || agreements[0].URL == "" {
		t.Fatalf("agreements = %#v, want content and url fields", agreements)
	}
}

func TestServerReturnsAgreementsWithoutAuthentication(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreements([]service.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
	})
	server := NewServer(svc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/agreements/current", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var agreements []service.AgreementDocument
	if err := json.NewDecoder(rec.Body).Decode(&agreements); err != nil {
		t.Fatalf("decode agreements: %v", err)
	}
	if len(agreements) != 2 {
		t.Fatalf("agreements = %#v, want public current documents", agreements)
	}
}

func TestServerAcceptsAgreements(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"agreements": []map[string]string{
			{
				"key":     "recharge_service",
				"version": "v1",
				"title":   "Recharge",
				"content": "Funds are used for official model calls.",
				"url":     "https://example.com/recharge",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/agreements/accept", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServerRejectsAgreementAcceptanceWhenPublishedContentDiffers(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"agreements": []map[string]string{
			{
				"key":     "recharge_service",
				"version": "v1",
				"title":   "Recharge",
				"content": "Different text",
				"url":     "https://example.com/recharge",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/agreements/accept", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServerRejectsUnknownAgreementAcceptance(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{Key: "recharge_service", Version: "v1", Title: "Recharge"})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"agreements": []map[string]string{
			{"key": "other", "version": "v1"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/agreements/accept", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestServerRejectsOrderWhenAgreementsMissing(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{Key: "recharge_service", Version: "v1", Title: "Recharge"})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(service.CreateOrderInput{AmountFen: 8800, Channel: "easypay"})
	req := httptest.NewRequest(http.MethodPost, "/wallet/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestServerRejectsOrderWhenAgreementContentChangesWithoutVersionBump(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	acceptBody, _ := json.Marshal(map[string]any{
		"agreements": []map[string]string{
			{
				"key":     "recharge_service",
				"version": "v1",
				"title":   "Recharge",
				"content": "Funds are used for official model calls.",
				"url":     "https://example.com/recharge",
			},
		},
	})
	acceptReq := httptest.NewRequest(http.MethodPost, "/agreements/accept", bytes.NewReader(acceptBody))
	acceptReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(acceptReq)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	server.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept status = %d, want %d", acceptRec.Code, http.StatusOK)
	}

	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls and account maintenance.",
		URL:     "https://example.com/recharge",
	})

	orderBody, _ := json.Marshal(service.CreateOrderInput{AmountFen: 8800, Channel: "easypay"})
	orderReq := httptest.NewRequest(http.MethodPost, "/wallet/orders", bytes.NewReader(orderBody))
	orderReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(orderReq)
	orderReq.Header.Set("Content-Type", "application/json")
	orderRec := httptest.NewRecorder()
	server.ServeHTTP(orderRec, orderReq)

	if orderRec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", orderRec.Code, http.StatusForbidden)
	}
}

func TestServerCreatesRefundRequest(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 1200, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{"order_id": order.ID, "amount_fen": 200, "reason": "test"})
	req := httptest.NewRequest(http.MethodPost, "/wallet/refund-requests", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestAdminRefundApprovalRequiresAdminAccessAndWorks(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 1200, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	refund, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "test")
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	body, _ := json.Marshal(map[string]any{"review_note": "approved"})
	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAdminRefundApproveFallsBackToPendingPayoutForManualProvider(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 1200, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	refund, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "test")
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got service.RefundRequest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "approved_pending_payout" {
		t.Fatalf("refund status = %q, want approved_pending_payout", got.Status)
	}
}

func TestAdminRefundSettleCompletesManualPayout(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 1200, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	refund, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "test")
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	if _, err := svc.ApproveRefundRequest(context.Background(), refund.ID, service.RefundDecisionInput{ReviewedBy: "admin"}); err != nil {
		t.Fatalf("ApproveRefundRequest() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	body, _ := json.Marshal(map[string]any{"external_refund_id": "manual-1", "external_status": "settled"})
	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/settle", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got service.RefundRequest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "refunded" || got.ExternalRefundID != "manual-1" {
		t.Fatalf("refund = %#v, want settled refunded request", got)
	}
}

func TestAdminRefundApprovalAllowsEmptyBody(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 1200, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	refund, err := svc.CreateRefundRequest(context.Background(), "user-1", 100, order.ID, "test")
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAdminOrderReconcileReturnsUpdatedOrder(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.SetPaymentProvider(stubPaymentProvider{
		queryResult: payments.OrderStatusResult{
			OrderID:         "ord-1",
			ExternalOrderID: "trade-1",
			Status:          "paid",
			ProviderStatus:  "TRADE_SUCCESS",
			Paid:            true,
			LastCheckedUnix: 10,
		},
	}); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID:          "ord-1",
		UserID:      "user-1",
		AmountFen:   500,
		Status:      "pending",
		Provider:    "stubpay",
		CreatedUnix: 1,
		UpdatedUnix: 1,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/ord-1/reconcile", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Changed bool                  `json:"changed"`
		Order   service.RechargeOrder `json:"order"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Changed || body.Order.Status != "paid" {
		t.Fatalf("response = %#v, want changed paid order", body)
	}
}

func TestAdminOrderReconcileReturnsNotImplementedForManualProvider(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID:          "ord-1",
		UserID:      "user-1",
		AmountFen:   500,
		Status:      "pending",
		Provider:    "manual",
		CreatedUnix: 1,
		UpdatedUnix: 1,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/ord-1/reconcile", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestAdminOrdersSupportFiltersAndPagination(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	for _, order := range []service.RechargeOrder{
		{ID: "ord-1", UserID: "user-1", Status: "pending", Provider: "easypay", AmountFen: 100, CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "ord-2", UserID: "user-1", Status: "paid", Provider: "manual", AmountFen: 200, CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "ord-3", UserID: "user-2", Status: "paid", Provider: "manual", AmountFen: 300, CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder(%s) error = %v", order.ID, err)
		}
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/orders?user_id=user-1&status=paid&provider=manual&limit=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []service.RechargeOrder
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].ID != "ord-2" {
		t.Fatalf("items = %#v, want only ord-2", items)
	}
}

func TestAdminBusinessCollectionsExposeUserNumbers(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID: "ord-detail", UserID: "user-2", Status: "paid", Provider: "manual", AmountFen: 500, CreatedUnix: 300, UpdatedUnix: 300,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.AppendTransaction(context.Background(), service.WalletTransaction{
		ID: "tx-detail", UserID: "user-2", Kind: "credit", AmountFen: 500, Description: "topup", CreatedUnix: 310,
	}); err != nil {
		t.Fatalf("AppendTransaction() error = %v", err)
	}
	if err := store.CreateRefundRequest(context.Background(), service.RefundRequest{
		ID: "refund-detail", UserID: "user-2", OrderID: "ord-detail", AmountFen: 120, Status: "pending", CreatedUnix: 340, UpdatedUnix: 340,
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	if err := store.CreateInfringementReport(context.Background(), service.InfringementReport{
		ID: "ipr-detail", UserID: "user-2", Subject: "copyright", Description: "reported", Status: "pending", CreatedUnix: 350, UpdatedUnix: 350,
	}); err != nil {
		t.Fatalf("CreateInfringementReport() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	t.Run("orders", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/orders?user_id=user-2", nil)
		req.Header.Set("Authorization", "Bearer token")
		addAdminSessionCookie(req)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var items []service.RechargeOrder
		if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(items) != 1 || items[0].UserNo == 0 {
			t.Fatalf("items = %#v, want user_no in orders payload", items)
		}
	})

	t.Run("wallet", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/wallet-adjustments?user_id=user-2", nil)
		req.Header.Set("Authorization", "Bearer token")
		addAdminSessionCookie(req)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var items []service.WalletTransaction
		if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(items) != 1 || items[0].UserNo == 0 {
			t.Fatalf("items = %#v, want user_no in wallet payload", items)
		}
	})

	t.Run("refunds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/refund-requests?user_id=user-2", nil)
		req.Header.Set("Authorization", "Bearer token")
		addAdminSessionCookie(req)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var items []service.RefundRequest
		if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(items) != 1 || items[0].UserNo == 0 {
			t.Fatalf("items = %#v, want user_no in refunds payload", items)
		}
	})

	t.Run("infringement", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/infringement-reports?user_id=user-2", nil)
		req.Header.Set("Authorization", "Bearer token")
		addAdminSessionCookie(req)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var items []service.InfringementReport
		if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(items) != 1 || items[0].UserNo == 0 {
			t.Fatalf("items = %#v, want user_no in infringement payload", items)
		}
	})
}

func TestAdminDashboardSupportsCustomWindowDays(t *testing.T) {
	store := &dashboardSummaryStoreForAPI{MemoryStore: service.NewMemoryStore()}
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	before := time.Now().Unix()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard?since_days=30", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	after := time.Now().Unix()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !store.called {
		t.Fatal("expected dashboard summary store to be used")
	}
	minExpected := before - 30*24*3600
	maxExpected := after - 30*24*3600
	if store.lastInput.SinceUnix < minExpected || store.lastInput.SinceUnix > maxExpected {
		t.Fatalf("since_unix = %d, want between %d and %d", store.lastInput.SinceUnix, minExpected, maxExpected)
	}
}

func TestAdminUsersSupportFiltersAndPagination(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	store.SetBalance("user-1", 100)
	store.SetBalance("user-2", 200)
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?user_id=user-2", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []service.UserSummary
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-2" {
		t.Fatalf("items = %#v, want only user-2", items)
	}
}

func TestAdminUsersSupportEmailFilter(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID:       "user-9",
		Email:        "newuser@example.com",
		CreatedUnix:  100,
		UpdatedUnix:  150,
		LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?email=newuser@example.com", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []service.UserSummary
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].Email != "newuser@example.com" {
		t.Fatalf("items = %#v, want email filtered user", items)
	}
	if items[0].CreatedUnix != 100 || items[0].LastSeenUnix != 200 {
		t.Fatalf("items = %#v, want profile timestamps", items)
	}
}

func TestAdminUsersSupportKeywordUserNumberFilter(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	for _, identity := range []service.UserIdentity{
		{UserID: "user-9", Username: "阿星", Email: "newuser@example.com", CreatedUnix: 100, UpdatedUnix: 150, LastSeenUnix: 200},
		{UserID: "user-10", Username: "测试二号", Email: "other@example.com", CreatedUnix: 101, UpdatedUnix: 160, LastSeenUnix: 210},
	} {
		if err := svc.UpsertUserIdentity(context.Background(), identity); err != nil {
			t.Fatalf("UpsertUserIdentity(%s) error = %v", identity.UserID, err)
		}
	}
	allItems, err := svc.ListUsers(context.Background(), service.UserSummaryFilter{})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(allItems) != 2 {
		t.Fatalf("len(allItems) = %d, want 2", len(allItems))
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?keyword="+url.QueryEscape(strconv.FormatInt(allItems[0].UserNo, 10)), nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []service.UserSummary
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].UserID != allItems[0].UserID {
		t.Fatalf("items = %#v, want only keyword-matched user", items)
	}
	if items[0].UserNo == 0 {
		t.Fatalf("items = %#v, want user_no in API payload", items)
	}

	nameReq := httptest.NewRequest(http.MethodGet, "/admin/users?keyword="+url.QueryEscape("阿星"), nil)
	nameReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(nameReq)
	nameRec := httptest.NewRecorder()
	server.ServeHTTP(nameRec, nameReq)
	if nameRec.Code != http.StatusOK {
		t.Fatalf("username status = %d, want %d: %s", nameRec.Code, http.StatusOK, nameRec.Body.String())
	}
	items = nil
	if err := json.NewDecoder(nameRec.Body).Decode(&items); err != nil {
		t.Fatalf("decode username response: %v", err)
	}
	if len(items) != 1 || items[0].Username != "阿星" {
		t.Fatalf("items = %#v, want username keyword result", items)
	}
}

func TestAdminMeReturnsRoleAndPermissions(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp struct {
		User     AuthUser              `json:"user"`
		Operator service.AdminOperator `json:"operator"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.User.ID != "root-1" || resp.Operator.Role != service.AdminRoleSuperAdmin {
		t.Fatalf("response = %#v, want current user + super-admin operator", resp)
	}
	if !resp.Operator.HasCapability(service.AdminCapabilityDashboardRead) || !resp.Operator.HasCapability(service.AdminCapabilityWalletWrite) {
		t.Fatalf("operator = %#v, want capability payload for admin UI", resp.Operator)
	}
}

func TestAdminDashboardReturnsTotalsAndTopModels(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	now := time.Now()
	baseUnix := now.Unix()
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	store.SetBalance("user-1", 400)
	store.SetBalance("user-2", 700)
	for _, identity := range []service.UserIdentity{
		{UserID: "user-1", Email: "one@example.com", CreatedUnix: now.Add(-10 * 24 * time.Hour).Unix(), UpdatedUnix: baseUnix - 3600, LastSeenUnix: baseUnix - 3600},
		{UserID: "user-2", Email: "two@example.com", CreatedUnix: now.Add(-2 * 24 * time.Hour).Unix(), UpdatedUnix: baseUnix - 1800, LastSeenUnix: baseUnix - 1800},
	} {
		if err := svc.UpsertUserIdentity(context.Background(), identity); err != nil {
			t.Fatalf("UpsertUserIdentity(%s) error = %v", identity.UserID, err)
		}
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID: "ord-1", UserID: "user-1", Status: "paid", Provider: "manual", AmountFen: 500,
		CreatedUnix: now.Add(-2 * 24 * time.Hour).Unix(), UpdatedUnix: now.Add(-2 * 24 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.CreateRefundRequest(context.Background(), service.RefundRequest{
		ID: "refund-1", UserID: "user-1", OrderID: "ord-1", AmountFen: 120, Status: "pending",
		CreatedUnix: now.Add(-12 * time.Hour).Unix(), UpdatedUnix: now.Add(-12 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	for _, usage := range []service.ChatUsageRecord{
		{ID: "usage-1", UserID: "user-1", ModelID: "official-basic", ChargedFen: 30, CreatedUnix: now.Add(-2 * 24 * time.Hour).Unix()},
		{ID: "usage-2", UserID: "user-2", ModelID: "official-pro", ChargedFen: 80, CreatedUnix: now.Add(-12 * time.Hour).Unix()},
	} {
		if err := store.RecordChatUsage(context.Background(), usage); err != nil {
			t.Fatalf("RecordChatUsage(%s) error = %v", usage.ID, err)
		}
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var dashboard service.AdminDashboard
	if err := json.NewDecoder(rec.Body).Decode(&dashboard); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if dashboard.Totals.Users != 2 || dashboard.Totals.PaidOrders != 1 || dashboard.Totals.WalletBalanceFen != 1100 || dashboard.Totals.RefundPending != 1 {
		t.Fatalf("dashboard totals = %#v, want aggregated admin metrics", dashboard.Totals)
	}
	if len(dashboard.TopModels) == 0 || dashboard.TopModels[0].ModelID != "official-pro" {
		t.Fatalf("top_models = %#v, want ranked recent model usage", dashboard.TopModels)
	}
}

func TestAdminUserDetailEndpointsReturnOverviewAndUsage(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	store.SetBalance("user-2", 880)
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID: "ord-detail", UserID: "user-2", Status: "paid", Provider: "manual", AmountFen: 500, CreatedUnix: 300, UpdatedUnix: 300,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.AppendTransaction(context.Background(), service.WalletTransaction{
		ID: "tx-detail", UserID: "user-2", Kind: "credit", AmountFen: 500, Description: "topup", CreatedUnix: 310,
	}); err != nil {
		t.Fatalf("AppendTransaction() error = %v", err)
	}
	if err := store.RecordAgreementAcceptance(context.Background(), service.AgreementAcceptance{
		UserID: "user-2", AgreementKey: "recharge_service", Version: "v1", AcceptedUnix: 320,
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptance() error = %v", err)
	}
	if err := store.RecordChatUsage(context.Background(), service.ChatUsageRecord{
		ID: "usage-detail", UserID: "user-2", ModelID: "official-pro", ChargedFen: 66, CreatedUnix: 330,
	}); err != nil {
		t.Fatalf("RecordChatUsage() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "user@example.com"}, nil, nil)

	overviewReq := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/overview", nil)
	overviewReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(overviewReq)
	overviewRec := httptest.NewRecorder()
	server.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("overview status = %d, want %d: %s", overviewRec.Code, http.StatusOK, overviewRec.Body.String())
	}
	var overview service.AdminUserOverview
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.User.UserID != "user-2" || overview.Wallet.BalanceFen != 880 || len(overview.RecentOrders) != 1 {
		t.Fatalf("overview = %#v, want populated admin user overview", overview)
	}
	if overview.User.UserNo == 0 {
		t.Fatalf("overview = %#v, want user_no in overview payload", overview)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(usageReq)
	usageRec := httptest.NewRecorder()
	server.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("usage status = %d, want %d: %s", usageRec.Code, http.StatusOK, usageRec.Body.String())
	}
	var usage []service.ChatUsageRecord
	if err := json.NewDecoder(usageRec.Body).Decode(&usage); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	if len(usage) != 1 || usage[0].ID != "usage-detail" {
		t.Fatalf("usage = %#v, want user usage history", usage)
	}
}

func TestAdminUserOverviewSerializesNilCollectionsAsArrays(t *testing.T) {
	store := &nilOverviewSliceStore{MemoryStore: service.NewMemoryStore()}
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "user@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/overview", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	requireJSONFieldArrayLength(t, rec.Body.Bytes(), "recent_orders", 0)
	requireJSONFieldArrayLength(t, rec.Body.Bytes(), "recent_transactions", 0)
	requireJSONFieldArrayLength(t, rec.Body.Bytes(), "agreements", 0)
	requireJSONFieldArrayLength(t, rec.Body.Bytes(), "recent_usage", 0)
}

func TestAdminUserDetailEndpointsRequireScopedCapabilities(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "governance@example.com",
		Role:   service.AdminRoleGovernance,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	store.SetBalance("user-2", 880)
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), service.RechargeOrder{
		ID: "ord-detail", UserID: "user-2", Status: "paid", Provider: "manual", AmountFen: 500, CreatedUnix: 300, UpdatedUnix: 300,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.AppendTransaction(context.Background(), service.WalletTransaction{
		ID: "tx-detail", UserID: "user-2", Kind: "credit", AmountFen: 500, Description: "topup", CreatedUnix: 310,
	}); err != nil {
		t.Fatalf("AppendTransaction() error = %v", err)
	}
	if err := store.RecordAgreementAcceptance(context.Background(), service.AgreementAcceptance{
		UserID: "user-2", AgreementKey: "recharge_service", Version: "v1", AcceptedUnix: 320,
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptance() error = %v", err)
	}
	if err := store.RecordChatUsage(context.Background(), service.ChatUsageRecord{
		ID: "usage-detail", UserID: "user-2", ModelID: "official-pro", ChargedFen: 66, CreatedUnix: 330,
	}); err != nil {
		t.Fatalf("RecordChatUsage() error = %v", err)
	}
	if err := store.CreateRefundRequest(context.Background(), service.RefundRequest{
		ID: "refund-detail", UserID: "user-2", OrderID: "ord-detail", AmountFen: 120, Status: "pending", CreatedUnix: 340, UpdatedUnix: 340,
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	if err := store.CreateInfringementReport(context.Background(), service.InfringementReport{
		ID: "ipr-detail", UserID: "user-2", Subject: "copyright", Description: "reported", Status: "pending", CreatedUnix: 350, UpdatedUnix: 350,
	}); err != nil {
		t.Fatalf("CreateInfringementReport() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "gov-1", email: "governance@example.com"}, nil, nil)

	overviewReq := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/overview", nil)
	overviewReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(overviewReq)
	overviewRec := httptest.NewRecorder()
	server.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("overview status = %d, want %d: %s", overviewRec.Code, http.StatusOK, overviewRec.Body.String())
	}
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "recent_orders", 0)
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "recent_transactions", 0)
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "recent_usage", 0)
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "agreements", 1)
	var overview service.AdminUserOverview
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.Wallet.BalanceFen != 0 || len(overview.RecentTransactions) != 0 || len(overview.RecentOrders) != 0 || len(overview.RecentUsage) != 0 {
		t.Fatalf("overview = %#v, want wallet/orders/usage redacted without scoped capabilities", overview)
	}
	if len(overview.Agreements) != 1 || overview.PendingRefundCount != 1 || overview.PendingInfringementCount != 1 {
		t.Fatalf("overview = %#v, want agreements and governance counts preserved", overview)
	}

	for _, tc := range []struct {
		path string
		name string
	}{
		{path: "/admin/users/user-2/wallet-transactions", name: "wallet transactions"},
		{path: "/admin/users/user-2/orders", name: "orders"},
		{path: "/admin/users/user-2/usage", name: "usage"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer token")
		addAdminSessionCookie(req)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want %d: %s", tc.name, rec.Code, http.StatusForbidden, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "缺少所需管理员权限") {
			t.Fatalf("%s body = %q, want localized capability denial", tc.name, rec.Body.String())
		}
	}
}

func TestAdminUserAgreementDataRequiresAgreementCapability(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "finance@example.com",
		Role:   service.AdminRoleFinance,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	store.SetBalance("user-2", 880)
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := store.RecordAgreementAcceptance(context.Background(), service.AgreementAcceptance{
		UserID: "user-2", AgreementKey: "recharge_service", Version: "v1", AcceptedUnix: 320, RemoteAddr: "203.0.113.1", DeviceSummary: "desktop",
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptance() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "finance-1", email: "finance@example.com"}, nil, nil)

	overviewReq := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/overview", nil)
	overviewReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(overviewReq)
	overviewRec := httptest.NewRecorder()
	server.ServeHTTP(overviewRec, overviewReq)
	if overviewRec.Code != http.StatusOK {
		t.Fatalf("overview status = %d, want %d: %s", overviewRec.Code, http.StatusOK, overviewRec.Body.String())
	}
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "agreements", 0)
	requireJSONFieldArrayLength(t, overviewRec.Body.Bytes(), "recent_usage", 0)
	var overview service.AdminUserOverview
	if err := json.NewDecoder(overviewRec.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if len(overview.Agreements) != 0 {
		t.Fatalf("overview agreements = %#v, want redacted agreements without agreements.read", overview.Agreements)
	}

	agreementsReq := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/agreements", nil)
	agreementsReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(agreementsReq)
	agreementsRec := httptest.NewRecorder()
	server.ServeHTTP(agreementsRec, agreementsReq)
	if agreementsRec.Code != http.StatusForbidden {
		t.Fatalf("agreements status = %d, want %d: %s", agreementsRec.Code, http.StatusForbidden, agreementsRec.Body.String())
	}
}

func TestAdminUserOverviewReturnsInternalServerErrorForStorageFailures(t *testing.T) {
	store := &failingOverviewStore{
		MemoryStore: service.NewMemoryStore(),
		walletErr:   errors.New("wallet unavailable"),
	}
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.UpsertUserIdentity(context.Background(), service.UserIdentity{
		UserID: "user-2", Email: "detail@example.com", CreatedUnix: 100, UpdatedUnix: 200, LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "root-1", email: "root@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users/user-2/overview", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestAdminUserOverviewReturnsNotFoundForMissingUser(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users/missing/overview", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminOperatorsCanBeListedAndUpdated(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com", "reader@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	updateBody, _ := json.Marshal(map[string]any{
		"role":   service.AdminRoleReadOnly,
		"active": true,
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/admin/operators/reader%40example.com", bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(updateReq)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	var updated service.AdminOperator
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated operator: %v", err)
	}
	if updated.Email != "reader@example.com" || updated.Role != service.AdminRoleReadOnly {
		t.Fatalf("updated = %#v, want saved read-only operator", updated)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/operators", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(listReq)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var operators []service.AdminOperator
	if err := json.NewDecoder(listRec.Body).Decode(&operators); err != nil {
		t.Fatalf("decode operator list: %v", err)
	}
	if len(operators) != 2 {
		t.Fatalf("operators = %#v, want two admin operators", operators)
	}
}

func TestAdminOperatorUpdateRejectsUnknownRole(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com", "reader@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	updateBody, _ := json.Marshal(map[string]any{
		"role":   "root-plus",
		"active": true,
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/admin/operators/reader%40example.com", bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(updateReq)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d: %s", updateRec.Code, http.StatusBadRequest, updateRec.Body.String())
	}
	if !strings.Contains(updateRec.Body.String(), service.ErrInvalidAdminRole.Error()) {
		t.Fatalf("body = %q, want invalid admin role error", updateRec.Body.String())
	}
}

func TestAdminWriteEndpointRejectsReadOnlyOperator(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"root@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "user@example.com",
		Role:   service.AdminRoleReadOnly,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "user@example.com"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"user_id":     "user-2",
		"amount_fen":  500,
		"description": "manual credit",
		"request_id":  "adjustment-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/wallet-adjustments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestCreateInfringementReportRejectsUnsafeEvidenceURL(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"subject":       "copyright issue",
		"description":   "unsafe evidence link",
		"evidence_urls": []string{"javascript:alert(1)"},
	})
	req := httptest.NewRequest(http.MethodPost, "/infringement-reports", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid evidence url") {
		t.Fatalf("body = %q, want invalid evidence url error", rec.Body.String())
	}
}

func TestAdminModelsPutUpdatesCatalogAndAudits(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	manager := runtimeconfig.NewManager(
		filepath.Join(t.TempDir(), "platform-runtime.json"),
		svc,
		upstream.NewRouter(nil),
	)
	if err := manager.Bootstrap(runtimeconfig.State{}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, manager)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(getReq)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("models get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	revision := strings.TrimSpace(getRec.Header().Get("ETag"))
	if revision == "" {
		t.Fatal("expected admin models GET to return an ETag revision")
	}

	body, _ := json.Marshal([]service.OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: false},
		{ID: "official-pro", Name: "Official Pro", Enabled: false},
	})
	req := httptest.NewRequest(http.MethodPut, "/admin/models", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", revision)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var models []service.OfficialModel
	if err := json.NewDecoder(rec.Body).Decode(&models); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(models) != 2 || models[0].ID != "official-basic" {
		t.Fatalf("models = %#v, want updated model catalog", models)
	}

	logs, err := svc.ListAuditLogs(context.Background(), service.AuditLogFilter{Action: "admin.models.updated"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v, want one admin models audit log", logs)
	}
}

func TestAdminWalletAdjustmentsSupportFilters(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := store.Credit(context.Background(), "user-1", 100, "credit 1"); err != nil {
		t.Fatalf("Credit(user-1) error = %v", err)
	}
	if _, err := store.Credit(context.Background(), "user-2", 200, "credit 2"); err != nil {
		t.Fatalf("Credit(user-2) error = %v", err)
	}
	if _, err := store.Debit(context.Background(), "user-2", 50, "debit 2"); err != nil {
		t.Fatalf("Debit(user-2) error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/wallet-adjustments?user_id=user-2&kind=debit", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var items []service.WalletTransaction
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-2" || items[0].Kind != "debit" {
		t.Fatalf("items = %#v, want only user-2 debit", items)
	}
}

func TestRefundRequestListEndpointsSupportFilters(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	for _, request := range []service.RefundRequest{
		{ID: "refund-1", UserID: "user-1", OrderID: "ord-1", Status: "pending", AmountFen: 100, CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "refund-2", UserID: "user-1", OrderID: "ord-2", Status: "refunded", AmountFen: 200, CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "refund-3", UserID: "user-2", OrderID: "ord-3", Status: "approved_pending_payout", AmountFen: 300, CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.CreateRefundRequest(context.Background(), request); err != nil {
			t.Fatalf("CreateRefundRequest(%s) error = %v", request.ID, err)
		}
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	userReq := httptest.NewRequest(http.MethodGet, "/wallet/refund-requests?status=refunded", nil)
	userReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(userReq)
	userRec := httptest.NewRecorder()
	server.ServeHTTP(userRec, userReq)

	if userRec.Code != http.StatusOK {
		t.Fatalf("user status = %d, want %d: %s", userRec.Code, http.StatusOK, userRec.Body.String())
	}
	var userItems []service.RefundRequest
	if err := json.NewDecoder(userRec.Body).Decode(&userItems); err != nil {
		t.Fatalf("decode user response: %v", err)
	}
	if len(userItems) != 1 || userItems[0].ID != "refund-2" {
		t.Fatalf("user items = %#v, want only refund-2", userItems)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/refund-requests?status=approved_pending_payout&user_id=user-2", nil)
	adminReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(adminReq)
	adminRec := httptest.NewRecorder()
	server.ServeHTTP(adminRec, adminReq)

	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d: %s", adminRec.Code, http.StatusOK, adminRec.Body.String())
	}
	var adminItems []service.RefundRequest
	if err := json.NewDecoder(adminRec.Body).Decode(&adminItems); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	if len(adminItems) != 1 || adminItems[0].ID != "refund-3" {
		t.Fatalf("admin items = %#v, want only refund-3", adminItems)
	}
}

func TestInfringementReportListEndpointsSupportFilters(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	for _, report := range []service.InfringementReport{
		{ID: "ipr-1", UserID: "user-1", Subject: "s1", Description: "d1", Status: "pending", CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "ipr-2", UserID: "user-1", Subject: "s2", Description: "d2", Status: "resolved", ReviewedBy: "admin-1", CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "ipr-3", UserID: "user-2", Subject: "s3", Description: "d3", Status: "reviewing", ReviewedBy: "admin-2", CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.CreateInfringementReport(context.Background(), report); err != nil {
			t.Fatalf("CreateInfringementReport(%s) error = %v", report.ID, err)
		}
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	userReq := httptest.NewRequest(http.MethodGet, "/infringement-reports?status=resolved", nil)
	userReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(userReq)
	userRec := httptest.NewRecorder()
	server.ServeHTTP(userRec, userReq)

	if userRec.Code != http.StatusOK {
		t.Fatalf("user status = %d, want %d: %s", userRec.Code, http.StatusOK, userRec.Body.String())
	}
	var userItems []service.InfringementReport
	if err := json.NewDecoder(userRec.Body).Decode(&userItems); err != nil {
		t.Fatalf("decode user response: %v", err)
	}
	if len(userItems) != 1 || userItems[0].ID != "ipr-2" {
		t.Fatalf("user items = %#v, want only ipr-2", userItems)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/infringement-reports?status=reviewing&reviewed_by=admin-2", nil)
	adminReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(adminReq)
	adminRec := httptest.NewRecorder()
	server.ServeHTTP(adminRec, adminReq)

	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d: %s", adminRec.Code, http.StatusOK, adminRec.Body.String())
	}
	var adminItems []service.InfringementReport
	if err := json.NewDecoder(adminRec.Body).Decode(&adminItems); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	if len(adminItems) != 1 || adminItems[0].ID != "ipr-3" {
		t.Fatalf("admin items = %#v, want only ipr-3", adminItems)
	}
}

func TestServerProxiesOfficialChat(t *testing.T) {
	store := service.NewMemoryStore()
	store.SetBalance("user-1", 5000)
	svc := service.NewService(store, nil)
	svc.SetOfficialModels([]service.OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetPricingRules(map[string]service.PricingRule{
		"official-basic": {
			ModelID:          "official-basic",
			FallbackPriceFen: 8,
		},
	})
	svc.SetOfficialProxyClient(stubProxyClient{
		response: platformapi.ChatProxyResponse{
			ChargedFen: 8,
		},
	})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(platformapi.ChatProxyRequest{
		ModelID: "official-basic",
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/official", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServerAuthLogin(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-1",
			Email:       "user@example.com",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp platformapi.AuthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Session.AccessToken != "token-1" {
		t.Fatalf("access_token = %q, want %q", resp.Session.AccessToken, "token-1")
	}
}

func TestAdminSessionLoginSetsCookieAndSupportsCookieBackedSession(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if _, err := svc.SaveAdminOperator(context.Background(), service.AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, service.AdminOperator{
		Email:  "ops@example.com",
		Role:   service.AdminRoleSuperAdmin,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "ops-1", email: "ops@example.com"}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "ops-1",
			Email:       "ops@example.com",
			ExpiresAt:   time.Now().Add(time.Hour).Unix(),
		},
	}, nil)

	loginBody, _ := json.Marshal(platformapi.AuthRequest{Email: "ops@example.com", Password: "secret"})
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/session/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	server.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected admin session login to set a cookie")
	}
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == adminSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("cookies = %#v, want %q", cookies, adminSessionCookieName)
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("expected admin session cookie to be httpOnly")
	}
	if got := sessionCookie.Path; got != "/admin" {
		t.Fatalf("cookie path = %q, want /admin", got)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/admin/session", nil)
	sessionReq.AddCookie(sessionCookie)
	sessionRec := httptest.NewRecorder()
	server.ServeHTTP(sessionRec, sessionReq)

	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d: %s", sessionRec.Code, http.StatusOK, sessionRec.Body.String())
	}
	var payload struct {
		User     AuthUser              `json:"user"`
		Operator service.AdminOperator `json:"operator"`
	}
	if err := json.NewDecoder(sessionRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if payload.User.ID != "ops-1" || payload.User.Email != "ops@example.com" {
		t.Fatalf("user = %#v, want cookie-backed admin identity", payload.User)
	}
	if !payload.Operator.Active || payload.Operator.Email != "ops@example.com" {
		t.Fatalf("operator = %#v, want active operator payload", payload.Operator)
	}
}

func TestAdminSessionLogoutClearsCookie(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/session/logout", nil)
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected logout to return a clearing cookie")
	}
	if cookies[0].Name != adminSessionCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %#v, want cleared admin session cookie", cookies)
	}
}

func TestAdminSessionLoginSetsHttpOnlyCookieAndReturnsOperatorProfile(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "admin@example.com"}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "admin-1",
			Email:       "admin@example.com",
			ExpiresAt:   time.Now().Add(30 * time.Minute).Unix(),
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "admin@example.com", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/admin/session/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("body = %q, should not expose raw access tokens to the admin browser", rec.Body.String())
	}
	var resp adminSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode admin session response: %v", err)
	}
	if resp.User.ID != "admin-1" || resp.Operator.Email != "admin@example.com" {
		t.Fatalf("response = %#v, want admin bootstrap payload", resp)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected admin session cookie to be set")
	}
	if sessionCookie.Value != "token-1" {
		t.Fatalf("cookie value = %q, want %q", sessionCookie.Value, "token-1")
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("expected admin session cookie to be HttpOnly")
	}
	if sessionCookie.Path != "/admin" {
		t.Fatalf("cookie path = %q, want /admin", sessionCookie.Path)
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie sameSite = %v, want Strict", sessionCookie.SameSite)
	}
}

func TestAdminSessionResumeAcceptsCookieAuth(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "admin@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/session", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp adminSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode admin session response: %v", err)
	}
	if resp.User.ID != "admin-1" || resp.Operator.Email != "admin@example.com" {
		t.Fatalf("response = %#v, want cookie-authenticated admin bootstrap payload", resp)
	}
}

func TestAdminRouteRejectsBearerOnlyToken(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "admin@example.com"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
	req.Header.Set("Authorization", "Bearer admin-only-token")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "管理员登录已过期") {
		t.Fatalf("body = %q, want localized administrator session requirement", rec.Body.String())
	}
}

func TestAdminSessionLogoutClearsCookieWithoutRequiringBearerAuth(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/session/logout", nil)
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	var cleared *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			cleared = cookie
			break
		}
	}
	if cleared == nil {
		t.Fatal("expected admin logout to clear the session cookie")
	}
	if cleared.Value != "" || cleared.MaxAge != -1 {
		t.Fatalf("cleared cookie = %#v, want empty value and MaxAge=-1", cleared)
	}
}

func TestAdminSessionLogoutRejectsCrossSiteOrigin(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/session/logout", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAdminSessionRejectsNonAdminLoginAndClearsCookie(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1", email: "user@example.com"}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-1",
			Email:       "user@example.com",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/admin/session/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "需要管理员权限") {
		t.Fatalf("body = %q, want localized admin access denial", rec.Body.String())
	}
	var cleared *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			cleared = cookie
			break
		}
	}
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("cleared cookie = %#v, want cleared admin cookie on denied login", cleared)
	}
}

func TestAdminSessionInvalidCookieIsRejectedAndCleared(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{err: errors.New("bad token")}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/session", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "stale-token", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "管理员登录已失效") {
		t.Fatalf("body = %q, want localized invalid administrator session message", rec.Body.String())
	}
	var cleared *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			cleared = cookie
			break
		}
	}
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("cleared cookie = %#v, want cleared cookie for invalid admin session", cleared)
	}
}

func TestAdminCookieBackedWriteRejectsCrossSiteOrigin(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "admin@example.com"}, nil, nil)

	body, _ := json.Marshal(service.AdminManualRechargeInput{
		UserID:      "user-2",
		AmountFen:   120,
		Description: "manual top up",
		RequestID:   "req-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "管理员会话校验失败") {
		t.Fatalf("body = %q, want localized origin validation message", rec.Body.String())
	}
	wallet, err := svc.GetWallet(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 0 {
		t.Fatalf("balance = %d, want unchanged balance after rejected cross-site write", wallet.BalanceFen)
	}
}

func TestAdminCookieBackedWriteAllowsSameOrigin(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "admin@example.com"}, nil, nil)

	body, _ := json.Marshal(service.AdminManualRechargeInput{
		UserID:      "user-2",
		AmountFen:   120,
		Description: "manual top up",
		RequestID:   "req-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	wallet, err := svc.GetWallet(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 120 {
		t.Fatalf("balance = %d, want 120 after accepted same-origin write", wallet.BalanceFen)
	}
}

func TestServerAuthSignupRequiresCurrentAuthAgreements(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	svc.SetAgreements([]service.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
	})
	signupCalls := 0
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		signupCalls: &signupCalls,
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-1",
			Email:       "user@example.com",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if signupCalls != 0 {
		t.Fatalf("signupCalls = %d, want 0 because agreements should be validated before upstream signup", signupCalls)
	}
	if !strings.Contains(rec.Body.String(), "注册前请先阅读并同意当前注册协议") {
		t.Fatalf("body = %q, want actionable signup agreement guidance", rec.Body.String())
	}
}

func TestServerAuthSignupPersistsAcceptedAuthAgreements(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	svc.SetAgreements([]service.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
	})
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-1",
			Email:       "user@example.com",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{
		Email:    "user@example.com",
		Password: "secret",
		Username: "阿星",
		Agreements: []platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
			{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PinchBotDesktop/1.0")
	req.RemoteAddr = "127.0.0.1:23456"
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	items, err := store.ListAgreementAcceptances(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListAgreementAcceptances() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want two persisted signup agreement acceptances", items)
	}
	if items[0].RemoteAddr == "" || items[0].DeviceSummary == "" {
		t.Fatalf("items = %#v, want stored source metadata", items)
	}
}

func TestServerAuthSignupReturnsRecoverableSuccessWhenAgreementPersistenceFails(t *testing.T) {
	store := &failingAgreementStore{
		MemoryStore: service.NewMemoryStore(),
		failures:    1,
	}
	svc := service.NewService(store, nil)
	svc.SetAgreements([]service.AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
	})
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-1",
			Email:       "user@example.com",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{
		Email:    "user@example.com",
		Password: "secret",
		Username: "阿星",
		Agreements: []platformapi.AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
			{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PinchBotDesktop/1.0")
	req.RemoteAddr = "127.0.0.1:23456"
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp struct {
		Session               platformapi.Session `json:"session"`
		AgreementSyncRequired bool                `json:"agreement_sync_required"`
		Warning               string              `json:"warning"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode signup response: %v", err)
	}
	if resp.Session.AccessToken != "token-1" {
		t.Fatalf("session = %#v, want successful signup session", resp.Session)
	}
	if !resp.AgreementSyncRequired {
		t.Fatalf("response = %#v, want agreement_sync_required=true", resp)
	}
	if resp.Warning != "注册已成功，但协议确认同步失败，请在充值前重新确认协议" {
		t.Fatalf("warning = %q, want sanitized recoverable signup guidance", resp.Warning)
	}
	if strings.Contains(resp.Warning, "store unavailable") {
		t.Fatalf("warning = %q, should not leak internal persistence details", resp.Warning)
	}
}

func TestServerAuthLoginMirrorsUserIntoAdminList(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, stubAuthBridge{
		session: platformapi.Session{
			AccessToken: "token-1",
			UserID:      "user-9",
			Username:    "阿星",
			Email:       "newuser@example.com",
		},
	}, nil)

	loginBody, _ := json.Marshal(platformapi.AuthRequest{Email: "newuser@example.com", Password: "secret"})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	server.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginRec.Code, http.StatusOK)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/users?user_id=user-9", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(listReq)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var items []service.UserSummary
	if err := json.NewDecoder(listRec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-9" {
		t.Fatalf("items = %#v, want mirrored user", items)
	}
	if items[0].Email != "newuser@example.com" {
		t.Fatalf("items = %#v, want mirrored email", items)
	}
	if items[0].Username != "阿星" {
		t.Fatalf("items = %#v, want mirrored username", items)
	}
}

func TestServerReturnsCurrentUserProfile(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp platformapi.BrowserAuthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if resp.Session.UserID != "user-1" || resp.Session.Email != "user@example.com" {
		t.Fatalf("session = %#v, want current auth user profile", resp.Session)
	}
}

func TestServerLogoutReturnsNoContent(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestAdminWalletAdjustmentCreatesTaggedTransaction(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"user_id":     "user-2",
		"amount_fen":  500,
		"description": "manual credit",
		"request_id":  "adjustment-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/wallet-adjustments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var wallet service.WalletSummary
	if err := json.NewDecoder(rec.Body).Decode(&wallet); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if wallet.BalanceFen != 500 {
		t.Fatalf("wallet = %#v, want credited balance", wallet)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/wallet-adjustments?user_id=user-2&reference_type=admin_adjustment", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(listReq)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var txs []service.WalletTransaction
	if err := json.NewDecoder(listRec.Body).Decode(&txs); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(txs) != 1 || txs[0].ReferenceType != "admin_adjustment" {
		t.Fatalf("transactions = %#v, want tagged admin adjustment", txs)
	}
}

func TestAdminManualRechargeCreatesTaggedTransaction(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"user_id":     "user-2",
		"amount_fen":  600,
		"description": "admin grant",
		"request_id":  "recharge-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var wallet service.WalletSummary
	if err := json.NewDecoder(rec.Body).Decode(&wallet); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if wallet.BalanceFen != 600 {
		t.Fatalf("wallet = %#v, want credited balance", wallet)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/wallet-adjustments?user_id=user-2&reference_type=admin_manual_recharge", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(listReq)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var txs []service.WalletTransaction
	if err := json.NewDecoder(listRec.Body).Decode(&txs); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(txs) != 1 || txs[0].ReferenceType != "admin_manual_recharge" || txs[0].Kind != "credit" {
		t.Fatalf("transactions = %#v, want tagged admin manual recharge", txs)
	}
}

func TestAdminManualRechargeRejectsNonPositiveAmount(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"user_id":    "user-2",
		"amount_fen": 0,
		"request_id": "recharge-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminManualRechargeIsIdempotentByRequestID(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"user_id":     "user-2",
		"amount_fen":  600,
		"description": "admin grant",
		"request_id":  "recharge-1",
	})
	firstReq := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	firstReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(firstReq)
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	server.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d: %s", firstRec.Code, http.StatusCreated, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/admin/manual-recharges", bytes.NewReader(body))
	secondReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(secondReq)
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	server.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d: %s", secondRec.Code, http.StatusOK, secondRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/wallet-adjustments?user_id=user-2&reference_type=admin_manual_recharge", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(listReq)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var txs []service.WalletTransaction
	if err := json.NewDecoder(listRec.Body).Decode(&txs); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("transactions = %#v, want one manual recharge after idempotent replay", txs)
	}
}

func TestAdminAuditLogsSupportsRichFiltersAndCSVExport(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"auditor@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "admin-1", email: "auditor@example.com"}, nil, nil)

	for _, entry := range []service.AdminAuditLog{
		{ID: "audit-1", ActorUserID: "admin-1", ActorEmail: "auditor@example.com", Action: "admin.manual_recharge.created", TargetType: "wallet_account", TargetID: "user-2", RiskLevel: "high", Detail: "grant", CreatedUnix: 200},
		{ID: "audit-2", ActorUserID: "admin-2", ActorEmail: "other@example.com", Action: "admin.operator.updated", TargetType: "admin_operator", TargetID: "ops@example.com", RiskLevel: "medium", Detail: "role change", CreatedUnix: 100},
	} {
		if err := svc.RecordAdminAudit(context.Background(), entry); err != nil {
			t.Fatalf("RecordAdminAudit(%s) error = %v", entry.ID, err)
		}
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/admin/audit-logs?action=admin.manual_recharge.created&target_type=wallet_account&target_id=user-2&actor_user_id=admin-1&risk_level=high&since_unix=150", nil)
	jsonReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(jsonReq)
	jsonRec := httptest.NewRecorder()
	server.ServeHTTP(jsonRec, jsonReq)

	if jsonRec.Code != http.StatusOK {
		t.Fatalf("json status = %d, want %d: %s", jsonRec.Code, http.StatusOK, jsonRec.Body.String())
	}
	var items []service.AdminAuditLog
	if err := json.NewDecoder(jsonRec.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].ID != "audit-1" {
		t.Fatalf("items = %#v, want filtered high-risk audit log", items)
	}

	csvReq := httptest.NewRequest(http.MethodGet, "/admin/audit-logs?action=admin.manual_recharge.created&format=csv", nil)
	csvReq.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(csvReq)
	csvRec := httptest.NewRecorder()
	server.ServeHTTP(csvRec, csvReq)

	if csvRec.Code != http.StatusOK {
		t.Fatalf("csv status = %d, want %d: %s", csvRec.Code, http.StatusOK, csvRec.Body.String())
	}
	if contentType := csvRec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", contentType)
	}
	body := csvRec.Body.String()
	if !strings.Contains(body, "created_unix,actor_user_id,actor_email,action,target_type,target_id,risk_level,detail") {
		t.Fatalf("csv body = %q, want header row", body)
	}
	if !strings.Contains(body, "audit") && !strings.Contains(body, "admin.manual_recharge.created") {
		t.Fatalf("csv body = %q, want exported audit row", body)
	}
}

func TestServerAuthLoginLocalizesInvalidCredentialsError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: &platformapi.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid login credentials",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "邮箱或密码错误") {
		t.Fatalf("body = %q, want localized invalid-credentials error", rec.Body.String())
	}
}

func TestServerAuthLoginLocalizesInvalidJSONError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "请求格式错误") {
		t.Fatalf("body = %q, want localized invalid-json guidance", rec.Body.String())
	}
}

func TestAdminSessionLoginLocalizesInvalidCredentialsError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: &platformapi.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid login credentials",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "admin@example.com", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/admin/session/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "邮箱或密码错误") {
		t.Fatalf("body = %q, want localized invalid-credentials error", rec.Body.String())
	}
}

func TestAdminSessionLoginRejectsInvalidEmailBeforeAuthBridge(t *testing.T) {
	loginCalls := 0
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		loginCalls: &loginCalls,
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "bad-email", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/admin/session/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if loginCalls != 0 {
		t.Fatalf("loginCalls = %d, want 0 when email validation fails locally", loginCalls)
	}
	if !strings.Contains(rec.Body.String(), platformapi.InvalidEmailFormatMessage) {
		t.Fatalf("body = %q, want localized invalid-email-format guidance", rec.Body.String())
	}
}

func TestServerAuthSignupPreservesUserActionableError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: &platformapi.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Supabase signup did not return a session. Disable Confirm email or allow unverified email sign-ins.",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "关闭“Confirm email”") {
		t.Fatalf("body = %q, want localized actionable signup guidance", rec.Body.String())
	}
}

func TestServerAuthSignupLocalizesInvalidEmailFormatError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: &platformapi.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Unable to validate email address: invalid format",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "bad-email", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	bodyText := rec.Body.String()
	if strings.Contains(bodyText, "Unable to validate email address") {
		t.Fatalf("body = %q, want english Supabase error to be hidden", bodyText)
	}
	if !strings.Contains(bodyText, "邮箱格式不正确，请检查后重试") {
		t.Fatalf("body = %q, want localized invalid-email-format error", bodyText)
	}
}

func TestServerAuthSignupRejectsInvalidEmailBeforeAuthBridge(t *testing.T) {
	signupCalls := 0
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		signupCalls: &signupCalls,
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "bad-email", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if signupCalls != 0 {
		t.Fatalf("signupCalls = %d, want 0 when email validation fails locally", signupCalls)
	}
	if !strings.Contains(rec.Body.String(), platformapi.InvalidEmailFormatMessage) {
		t.Fatalf("body = %q, want localized invalid-email-format guidance", rec.Body.String())
	}
}

func TestServerAuthSignupSanitizesUnexpectedInternalErrors(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: errors.New("supabase auth bridge is not configured"),
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret", Username: "阿星"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	bodyText := rec.Body.String()
	if strings.Contains(bodyText, "supabase auth bridge is not configured") {
		t.Fatalf("body = %q, want sanitized error", bodyText)
	}
	if !strings.Contains(bodyText, "认证服务暂不可用") {
		t.Fatalf("body = %q, want localized auth service error", bodyText)
	}
}

func TestServerAuthLoginSanitizesUnexpectedInternalErrors(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: errors.New("dial tcp 10.0.0.1:443: connect: connection refused"),
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	bodyText := rec.Body.String()
	if strings.Contains(bodyText, "connection refused") || strings.Contains(bodyText, "10.0.0.1") {
		t.Fatalf("body = %q, want sanitized error", bodyText)
	}
	if !strings.Contains(bodyText, "认证服务暂不可用") {
		t.Fatalf("body = %q, want localized auth service error", bodyText)
	}
}

func TestServerAuthLoginRejectsMissingAccessToken(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		session: platformapi.Session{
			UserID:    "user-1",
			Email:     "user@example.com",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if !strings.Contains(rec.Body.String(), "未返回有效会话") {
		t.Fatalf("body = %q, want missing-session-token guidance", rec.Body.String())
	}
}

func TestServerSignupRequiresUsernameInChinese(t *testing.T) {
	signupCalls := 0
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		signupCalls: &signupCalls,
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if signupCalls != 0 {
		t.Fatalf("signupCalls = %d, want 0 when username validation fails", signupCalls)
	}
	if !strings.Contains(rec.Body.String(), "请输入用户名") {
		t.Fatalf("body = %q, want localized username guidance", rec.Body.String())
	}
}

func TestServerProtectedRouteSanitizesMissingVerifier(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(rec.Body.String(), "auth verifier not configured") {
		t.Fatalf("body = %q, want sanitized verifier error", rec.Body.String())
	}
}

func TestAdminModelsRequireAdminAccess(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	req.Header.Set("Authorization", "Bearer token")
	addAdminSessionCookie(req)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if !strings.Contains(rec.Body.String(), "需要管理员权限") {
		t.Fatalf("body = %q, want localized admin access denial", rec.Body.String())
	}
}

func TestEasyPayNotifyCreditsWallet(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SetPaymentProvider(makeTestEasyPayProvider()); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{
		AmountFen: 1200,
		Channel:   "easypay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	values := make(url.Values)
	values.Set("pid", "10001")
	values.Set("out_trade_no", order.ID)
	values.Set("trade_no", "trade_456")
	values.Set("type", "alipay")
	values.Set("name", "PinchBot Recharge")
	values.Set("money", "12.00")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("sign_type", "MD5")
	values.Set("sign", payments.NewEasyPayProvider(payments.EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	}).SignForTest(values))

	req := httptest.NewRequest(http.MethodPost, "/payments/easypay/notify", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 1200 {
		t.Fatalf("balance = %d, want 1200", wallet.BalanceFen)
	}
}

func TestEasyPayNotifyRejectsAmountMismatch(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SetPaymentProvider(makeTestEasyPayProvider()); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{
		AmountFen: 1200,
		Channel:   "easypay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	values := make(url.Values)
	values.Set("pid", "10001")
	values.Set("out_trade_no", order.ID)
	values.Set("trade_no", "trade_456")
	values.Set("type", "alipay")
	values.Set("name", "PinchBot Recharge")
	values.Set("money", "11.00")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("sign_type", "MD5")
	values.Set("sign", payments.NewEasyPayProvider(payments.EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	}).SignForTest(values))

	req := httptest.NewRequest(http.MethodPost, "/payments/easypay/notify", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 0 {
		t.Fatalf("balance = %d, want 0 after rejected callback", wallet.BalanceFen)
	}
}

func TestPaymentReturnReconcilesPaidOrder(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SetPaymentProvider(stubPaymentProvider{
		queryResult: payments.OrderStatusResult{
			OrderID:         "ord_test",
			ExternalOrderID: "trade_123",
			AmountFen:       8800,
			Status:          "paid",
			ProviderStatus:  "TRADE_SUCCESS",
			Paid:            true,
			LastCheckedUnix: time.Now().Unix(),
		},
	}); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{
		AmountFen: 8800,
		Channel:   "alimpay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/alimpay/return?out_trade_no="+url.QueryEscape(order.ID), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want html", contentType)
	}
	if !strings.Contains(rec.Body.String(), "支付成功") || !strings.Contains(rec.Body.String(), order.ID) {
		t.Fatalf("body = %q, want paid return page with order id", rec.Body.String())
	}
	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 8800 {
		t.Fatalf("balance = %d, want 8800", wallet.BalanceFen)
	}
	savedOrder, err := svc.GetOrder(context.Background(), "user-1", order.ID)
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if savedOrder.Status != "paid" || savedOrder.ExternalID != "trade_123" {
		t.Fatalf("order = %#v, want paid order with external id", savedOrder)
	}
}

func TestPaymentReturnRequiresOrderID(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/alimpay/return", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "缺少订单号参数 out_trade_no") {
		t.Fatalf("body = %q, want missing out_trade_no error", rec.Body.String())
	}
}

type stubVerifier struct {
	userID string
	email  string
	err    error
}

type failingAgreementStore struct {
	*service.MemoryStore
	failures int
}

type failingOverviewStore struct {
	*service.MemoryStore
	walletErr error
}

type nilOverviewSliceStore struct {
	*service.MemoryStore
}

type dashboardSummaryStoreForAPI struct {
	*service.MemoryStore
	lastInput service.AdminDashboardStoreInput
	called    bool
}

func (s *failingAgreementStore) RecordAgreementAcceptance(ctx context.Context, acceptance service.AgreementAcceptance) error {
	if s.failures > 0 {
		s.failures--
		return errors.New("agreement acceptance store unavailable")
	}
	return s.MemoryStore.RecordAgreementAcceptance(ctx, acceptance)
}

func (s *failingOverviewStore) GetWallet(ctx context.Context, userID string) (service.WalletSummary, error) {
	if s.walletErr != nil {
		return service.WalletSummary{}, s.walletErr
	}
	return s.MemoryStore.GetWallet(ctx, userID)
}

func (s *nilOverviewSliceStore) ListTransactions(ctx context.Context, userID string) ([]service.WalletTransaction, error) {
	return nil, nil
}

func (s *nilOverviewSliceStore) ListOrders(ctx context.Context) ([]service.RechargeOrder, error) {
	return nil, nil
}

func (s *nilOverviewSliceStore) ListAgreementAcceptances(ctx context.Context, userID string) ([]service.AgreementAcceptance, error) {
	return nil, nil
}

func (s *nilOverviewSliceStore) ListChatUsageRecords(ctx context.Context, filter service.ChatUsageRecordFilter) ([]service.ChatUsageRecord, error) {
	return nil, nil
}

func (s *dashboardSummaryStoreForAPI) BuildAdminDashboard(ctx context.Context, input service.AdminDashboardStoreInput) (service.AdminDashboard, error) {
	s.called = true
	s.lastInput = input
	return service.AdminDashboard{GeneratedUnix: time.Now().Unix()}, nil
}

func requireJSONFieldArrayLength(t *testing.T, body []byte, field string, wantLen int) {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode json body: %v", err)
	}
	raw, ok := payload[field]
	if !ok {
		t.Fatalf("field %q missing from payload: %s", field, string(body))
	}
	if strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		t.Fatalf("field %q serialized as null, want []: %s", field, string(body))
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("field %q should be a json array: %v (raw=%s)", field, err, string(raw))
	}
	if len(items) != wantLen {
		t.Fatalf("field %q length = %d, want %d (raw=%s)", field, len(items), wantLen, string(raw))
	}
}

func (s stubVerifier) Verify(ctx context.Context, bearerToken string) (AuthUser, error) {
	if s.err != nil {
		return AuthUser{}, s.err
	}
	email := strings.TrimSpace(s.email)
	if email == "" {
		email = "user@example.com"
	}
	return AuthUser{ID: s.userID, Email: email}, nil
}

type stubProxyClient struct {
	response platformapi.ChatProxyResponse
	err      error
}

func (s stubProxyClient) ProxyChat(ctx context.Context, userID string, request platformapi.ChatProxyRequest) (platformapi.ChatProxyResponse, error) {
	return s.response, s.err
}

func addAdminSessionCookie(req *http.Request) {
	if req == nil {
		return
	}
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: "token-1", Path: "/admin"})
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return
	}
	if req.Header.Get("Origin") != "" || req.Header.Get("Referer") != "" {
		return
	}
	scheme := req.URL.Scheme
	if scheme == "" {
		scheme = "http"
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if host == "" {
		host = "example.com"
	}
	req.Header.Set("Origin", scheme+"://"+host)
}

type stubAuthBridge struct {
	session       platformapi.Session
	err           error
	loginCalls    *int
	signupCalls   *int
	lastLoginReq  *platformapi.AuthRequest
	lastSignUpReq *platformapi.AuthRequest
}

func (s stubAuthBridge) Login(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	if s.loginCalls != nil {
		(*s.loginCalls)++
	}
	if s.lastLoginReq != nil {
		*s.lastLoginReq = req
	}
	return s.session, s.err
}

func (s stubAuthBridge) SignUp(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	if s.signupCalls != nil {
		(*s.signupCalls)++
	}
	if s.lastSignUpReq != nil {
		*s.lastSignUpReq = req
	}
	return s.session, s.err
}

type stubPaymentProvider struct {
	queryResult  payments.OrderStatusResult
	queryErr     error
	refundResult payments.RefundResult
	refundErr    error
}

func (s stubPaymentProvider) Name() string { return "stubpay" }

func (s stubPaymentProvider) Capabilities() payments.ProviderCapabilities {
	return payments.ProviderCapabilities{CanQueryOrder: true, CanRefund: true}
}

func (s stubPaymentProvider) CreateOrder(ctx context.Context, input payments.CreateOrderInput) (payments.PaymentOrder, error) {
	return payments.PaymentOrder{OrderID: input.OrderID, Status: "pending", Provider: "stubpay", AmountFen: input.AmountFen}, nil
}

func (s stubPaymentProvider) VerifyCallback(ctx context.Context, values url.Values) (payments.CallbackResult, error) {
	return payments.CallbackResult{}, nil
}

func (s stubPaymentProvider) QueryOrder(ctx context.Context, input payments.QueryOrderInput) (payments.OrderStatusResult, error) {
	return s.queryResult, s.queryErr
}

func (s stubPaymentProvider) Refund(ctx context.Context, input payments.RefundInput) (payments.RefundResult, error) {
	return s.refundResult, s.refundErr
}

func makeTestEasyPayProvider() payments.Provider {
	return payments.NewEasyPayProvider(payments.EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	})
}
