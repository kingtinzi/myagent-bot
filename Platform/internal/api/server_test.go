package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/platformapi"

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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestServerCreatesRechargeOrder(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(service.CreateOrderInput{AmountFen: 8800, Channel: "manual"})
	req := httptest.NewRequest(http.MethodPost, "/wallet/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/official/models", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestAdminRuntimeConfigRoundTrip(t *testing.T) {
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
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodGet, "/official/models", nil)
	req.Header.Set("Authorization", "Bearer token")
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/agreements/current", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestServerAcceptsAgreements(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	svc.SetAgreement(service.AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

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
	orderReq.Header.Set("Content-Type", "application/json")
	orderRec := httptest.NewRecorder()
	server.ServeHTTP(orderRec, orderReq)

	if orderRec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", orderRec.Code, http.StatusForbidden)
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

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
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

func TestServerAuthLoginPreservesUserActionableError(t *testing.T) {
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
	if !strings.Contains(rec.Body.String(), "Invalid login credentials") {
		t.Fatalf("body = %q, want user-actionable error", rec.Body.String())
	}
}

func TestAdminModelsRequireAdminAccess(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/models", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
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
	values.Set("name", "OpenClaw Recharge")
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
	values.Set("name", "OpenClaw Recharge")
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

type stubVerifier struct {
	userID string
	err    error
}

func (s stubVerifier) Verify(ctx context.Context, bearerToken string) (AuthUser, error) {
	if s.err != nil {
		return AuthUser{}, s.err
	}
	return AuthUser{ID: s.userID, Email: "user@example.com"}, nil
}

type stubProxyClient struct {
	response platformapi.ChatProxyResponse
	err      error
}

func (s stubProxyClient) ProxyChat(ctx context.Context, userID string, request platformapi.ChatProxyRequest) (platformapi.ChatProxyResponse, error) {
	return s.response, s.err
}

type stubAuthBridge struct {
	session platformapi.Session
	err     error
}

func (s stubAuthBridge) Login(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	return s.session, s.err
}

func (s stubAuthBridge) SignUp(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
	return s.session, s.err
}

func makeTestEasyPayProvider() payments.Provider {
	return payments.NewEasyPayProvider(payments.EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	})
}
