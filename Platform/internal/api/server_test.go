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
	"strings"
	"testing"

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

func TestServerReturnsOfficialAccessState(t *testing.T) {
	store := service.NewMemoryStore()
	store.SetBalance("user-1", 66)
	svc := service.NewService(store, nil)
	svc.SetOfficialModels([]service.OfficialModel{{ID: "official-basic", Name: "Official Basic", Enabled: true}})
	svc.SetPricingCatalog([]service.PricingRule{{ModelID: "official-basic", Version: "v20260313", FallbackPriceFen: 10}})
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/official/access", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestServerReturnsWalletOrder(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{AmountFen: 8800, Channel: "manual"})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet/orders/"+order.ID, nil)
	req.Header.Set("Authorization", "Bearer token")
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

	logs, err := svc.ListAuditLogs(context.Background(), service.AuditLogFilter{Action: "admin.runtime_config.updated"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected runtime config update to be audited")
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
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{"review_note": "approved"})
	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
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
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer token")
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
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	body, _ := json.Marshal(map[string]any{"external_refund_id": "manual-1", "external_status": "settled"})
	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/settle", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
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
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/refund-requests/"+refund.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAdminOrderReconcileReturnsUpdatedOrder(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/ord-1/reconcile", nil)
	req.Header.Set("Authorization", "Bearer token")
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
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/orders/ord-1/reconcile", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestAdminOrdersSupportFiltersAndPagination(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
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
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/orders?user_id=user-1&status=paid&provider=manual&limit=1", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestAdminUsersSupportFiltersAndPagination(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	store.SetBalance("user-1", 100)
	store.SetBalance("user-2", 200)
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/users?user_id=user-2", nil)
	req.Header.Set("Authorization", "Bearer token")
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

func TestServerAuthSignupPreservesUserActionableError(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: &platformapi.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Supabase signup did not return a session. Disable Confirm email or allow unverified email sign-ins.",
		},
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Disable Confirm email") {
		t.Fatalf("body = %q, want actionable signup guidance", rec.Body.String())
	}
}

func TestServerAuthSignupSanitizesUnexpectedInternalErrors(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, stubVerifier{}, stubAuthBridge{
		err: errors.New("supabase auth bridge is not configured"),
	}, nil)

	body, _ := json.Marshal(platformapi.AuthRequest{Email: "user@example.com", Password: "secret"})
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
	if !strings.Contains(bodyText, "authentication service unavailable") {
		t.Fatalf("body = %q, want generic auth service error", bodyText)
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
	if !strings.Contains(bodyText, "authentication service unavailable") {
		t.Fatalf("body = %q, want generic auth service error", bodyText)
	}
}

func TestServerProtectedRouteSanitizesMissingVerifier(t *testing.T) {
	svc := service.NewService(service.NewMemoryStore(), nil)
	server := NewServer(svc, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Authorization", "Bearer token")
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
