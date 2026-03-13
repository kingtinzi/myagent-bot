package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/sipeed/pinchbot/pkg/platformapi"
	"github.com/sipeed/pinchbot/pkg/providers/protocoltypes"

	"openclaw/platform/internal/payments"
)

func TestCreateRechargeOrderRequiresPositiveAmount(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)

	_, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 0,
		Channel:   "manual",
	})
	if err == nil {
		t.Fatal("expected error for non-positive amount")
	}
}

func TestProxyOfficialChatChargesWalletUsingUsage(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 5000)
	upstream := &stubChatClient{
		response: ChatResult{
			Content: "ok",
			Usage: Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		},
	}
	svc := NewService(store, upstream)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:                "official-basic",
			InputPriceMicrosPer1K:  10000,
			OutputPriceMicrosPer1K: 20000,
			FallbackPriceFen:       99,
		},
	})

	got, err := svc.ProxyOfficialChat(context.Background(), "user-1", ChatInput{
		ModelID: "official-basic",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("ProxyOfficialChat() error = %v", err)
	}
	if got.Content != "ok" {
		t.Fatalf("content = %q, want %q", got.Content, "ok")
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen >= 5000 {
		t.Fatalf("balance = %d, want less than 5000 after charge", wallet.BalanceFen)
	}
}

func TestProxyOfficialChatFallsBackWhenUsageMissing(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 5000)
	upstream := &stubChatClient{
		response: ChatResult{
			Content: "fallback",
		},
	}
	svc := NewService(store, upstream)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:          "official-basic",
			FallbackPriceFen: 37,
		},
	})

	_, err := svc.ProxyOfficialChat(context.Background(), "user-1", ChatInput{
		ModelID: "official-basic",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("ProxyOfficialChat() error = %v", err)
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 4963 {
		t.Fatalf("balance = %d, want 4963", wallet.BalanceFen)
	}
}

func TestProxyOfficialChatRequestChargesWallet(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 5000)
	svc := NewService(store, nil)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetOfficialProxyClient(stubOfficialProxyClient{
		response: platformapi.ChatProxyResponse{
			Response: protocoltypes.LLMResponse{
				Content: "proxy",
				Usage: &protocoltypes.UsageInfo{
					PromptTokens:     200,
					CompletionTokens: 100,
					TotalTokens:      300,
				},
			},
		},
	})
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:                "official-basic",
			InputPriceMicrosPer1K:  10000,
			OutputPriceMicrosPer1K: 20000,
			FallbackPriceFen:       99,
		},
	})

	got, err := svc.ProxyOfficialChatRequest(context.Background(), "user-1", platformapi.ChatProxyRequest{
		ModelID: "official-basic",
	})
	if err != nil {
		t.Fatalf("ProxyOfficialChatRequest() error = %v", err)
	}
	if got.Response.Content != "proxy" {
		t.Fatalf("content = %q, want %q", got.Response.Content, "proxy")
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen >= 5000 {
		t.Fatalf("balance = %d, want less than 5000 after charge", wallet.BalanceFen)
	}
}

func TestProxyOfficialChatRequestRequiresFallbackBalanceBeforeCallingUpstream(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 5)
	proxy := &recordingOfficialProxyClient{
		response: platformapi.ChatProxyResponse{
			Response: protocoltypes.LLMResponse{Content: "proxy"},
		},
	}
	svc := NewService(store, nil)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetOfficialProxyClient(proxy)
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:          "official-basic",
			FallbackPriceFen: 10,
		},
	})

	_, err := svc.ProxyOfficialChatRequest(context.Background(), "user-1", platformapi.ChatProxyRequest{
		ModelID: "official-basic",
	})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("error = %v, want ErrInsufficientFunds", err)
	}
	if proxy.called {
		t.Fatal("expected upstream proxy not to be called when balance is below fallback charge")
	}
}

func TestProxyOfficialChatRequestRefundsReservedChargeWhenUpstreamFails(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 50)
	svc := NewService(store, nil)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: true},
	})
	svc.SetOfficialProxyClient(stubOfficialProxyClient{
		err: errors.New("upstream unavailable"),
	})
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:          "official-basic",
			FallbackPriceFen: 10,
		},
	})

	_, err := svc.ProxyOfficialChatRequest(context.Background(), "user-1", platformapi.ChatProxyRequest{
		ModelID: "official-basic",
	})
	if err == nil {
		t.Fatal("expected upstream failure")
	}

	wallet, walletErr := svc.GetWallet(context.Background(), "user-1")
	if walletErr != nil {
		t.Fatalf("GetWallet() error = %v", walletErr)
	}
	if wallet.BalanceFen != 50 {
		t.Fatalf("balance = %d, want 50 after reserved charge refund", wallet.BalanceFen)
	}
}

func TestProxyOfficialChatRequestRejectsDisabledModel(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 5000)
	svc := NewService(store, nil)
	svc.SetOfficialModels([]OfficialModel{
		{ID: "official-basic", Name: "Official Basic", Enabled: false},
	})
	svc.SetPricingRules(map[string]PricingRule{
		"official-basic": {
			ModelID:          "official-basic",
			FallbackPriceFen: 10,
		},
	})
	svc.SetOfficialProxyClient(stubOfficialProxyClient{})

	_, err := svc.ProxyOfficialChatRequest(context.Background(), "user-1", platformapi.ChatProxyRequest{
		ModelID: "official-basic",
	})
	if err == nil {
		t.Fatal("expected disabled model to be rejected")
	}
	if !errors.Is(err, ErrModelDisabled) {
		t.Fatalf("error = %v, want ErrModelDisabled", err)
	}
}

func TestHandleSuccessfulRechargeCreditsWalletOnlyOnce(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 1200,
		Channel:   "easypay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}

	charged, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "easypay", "trade-1")
	if err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	if !charged {
		t.Fatal("expected first payment callback to credit wallet")
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 1200 {
		t.Fatalf("balance = %d, want 1200", wallet.BalanceFen)
	}

	charged, err = svc.HandleSuccessfulRecharge(context.Background(), order.ID, "easypay", "trade-1")
	if err != nil {
		t.Fatalf("HandleSuccessfulRecharge() second call error = %v", err)
	}
	if charged {
		t.Fatal("expected duplicate callback to be idempotent")
	}
}

func TestHandleSuccessfulRechargeDoesNotLeavePaidOrderWithoutBalanceCredit(t *testing.T) {
	store := &partialRechargeStore{
		order: RechargeOrder{
			ID:        "ord-1",
			UserID:    "user-1",
			AmountFen: 1200,
			Status:    "pending",
		},
		creditErr: errors.New("db unavailable"),
	}
	svc := NewService(store, nil)

	charged, err := svc.HandleSuccessfulRecharge(context.Background(), "ord-1", "easypay", "trade-1")
	if err == nil {
		t.Fatal("expected recharge handling to fail")
	}
	if charged {
		t.Fatal("expected failed credit to report no completed charge")
	}
	order, getErr := svc.GetOrder(context.Background(), "user-1", "ord-1")
	if getErr != nil {
		t.Fatalf("GetOrder() error = %v", getErr)
	}
	if order.Status != "pending" {
		t.Fatalf("order status = %q, want pending so callback can be retried safely", order.Status)
	}
}

func TestSyncAdminUsersRevokesRemovedEmails(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if err := svc.SyncAdminUsers(context.Background(), []string{"new-admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() second call error = %v", err)
	}

	oldAdmin, err := svc.IsAdminUser(context.Background(), "user-1", "admin@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() old admin error = %v", err)
	}
	if oldAdmin {
		t.Fatal("expected removed admin email to lose access")
	}

	newAdmin, err := svc.IsAdminUser(context.Background(), "user-2", "new-admin@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() new admin error = %v", err)
	}
	if !newAdmin {
		t.Fatal("expected new admin email to remain active")
	}
}

func TestIsAdminUser(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}

	ok, err := svc.IsAdminUser(context.Background(), "user-1", "admin@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() error = %v", err)
	}
	if !ok {
		t.Fatal("expected email allowlist user to be admin")
	}
}

func TestEnsureRechargeAgreementsAccepted(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreement(AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})

	err := svc.EnsureRechargeAgreementsAccepted(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected missing agreement acceptance to block recharge")
	}

	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{
			Key:     "recharge_service",
			Version: "v1",
			Title:   "Recharge",
			Content: "Funds are used for official model calls.",
			URL:     "https://example.com/recharge",
		},
	}, AgreementAcceptanceSource{}); err != nil {
		t.Fatalf("RecordAgreementAcceptances() error = %v", err)
	}

	if err := svc.EnsureRechargeAgreementsAccepted(context.Background(), "user-1"); err != nil {
		t.Fatalf("EnsureRechargeAgreementsAccepted() error = %v", err)
	}
}

func TestRecordAgreementAcceptancesRejectsUnknownAgreement(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreement(AgreementDocument{Key: "recharge_service", Version: "v1", Title: "Recharge"})

	err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{Key: "other", Version: "v1"},
	}, AgreementAcceptanceSource{})
	if !errors.Is(err, ErrUnknownAgreement) {
		t.Fatalf("RecordAgreementAcceptances() error = %v, want ErrUnknownAgreement", err)
	}
}

func TestRecordAgreementAcceptancesRejectsMismatchedPublishedContent(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreement(AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})

	err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{
			Key:     "recharge_service",
			Version: "v1",
			Title:   "Recharge",
			Content: "Different text",
			URL:     "https://example.com/recharge",
		},
	}, AgreementAcceptanceSource{})
	if !errors.Is(err, ErrInvalidAgreement) {
		t.Fatalf("RecordAgreementAcceptances() error = %v, want ErrInvalidAgreement", err)
	}
}

func TestEnsureRechargeAgreementsAcceptedRejectsContentDriftWithoutVersionBump(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreement(AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls.",
		URL:     "https://example.com/recharge",
	})

	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{
			Key:     "recharge_service",
			Version: "v1",
			Title:   "Recharge",
			Content: "Funds are used for official model calls.",
			URL:     "https://example.com/recharge",
		},
	}, AgreementAcceptanceSource{}); err != nil {
		t.Fatalf("RecordAgreementAcceptances() error = %v", err)
	}

	svc.SetAgreement(AgreementDocument{
		Key:     "recharge_service",
		Version: "v1",
		Title:   "Recharge",
		Content: "Funds are used for official model calls and account maintenance.",
		URL:     "https://example.com/recharge",
	})

	err := svc.EnsureRechargeAgreementsAccepted(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected changed published content with same version to require re-acceptance")
	}
}

func TestRecordAgreementAcceptancesStoresSourceMetadataAndHistory(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreements([]AgreementDocument{
		{Key: "recharge_service", Version: "v1", Title: "Recharge", Content: "v1", URL: "https://example.com/v1"},
		{Key: "recharge_service", Version: "v2", Title: "Recharge", Content: "v2", URL: "https://example.com/v2"},
	})

	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{Key: "recharge_service", Version: "v1", Title: "Recharge", Content: "v1", URL: "https://example.com/v1"},
	}, AgreementAcceptanceSource{
		ClientVersion: "launcher/1.0.0",
		RemoteAddr:    "127.0.0.1:12345",
		DeviceSummary: "pinchbot-test-agent",
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptances(v1) error = %v", err)
	}
	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{Key: "recharge_service", Version: "v2", Title: "Recharge", Content: "v2", URL: "https://example.com/v2"},
	}, AgreementAcceptanceSource{
		ClientVersion: "launcher/1.0.1",
		RemoteAddr:    "127.0.0.1:22345",
		DeviceSummary: "pinchbot-test-agent-2",
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptances(v2) error = %v", err)
	}

	items, err := store.ListAgreementAcceptances(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListAgreementAcceptances() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("acceptance count = %d, want 2", len(items))
	}
	foundMeta := false
	for _, item := range items {
		if item.ClientVersion == "launcher/1.0.1" && item.DeviceSummary == "pinchbot-test-agent-2" {
			foundMeta = true
		}
	}
	if !foundMeta {
		t.Fatalf("items = %#v, want stored client/device metadata", items)
	}
}

func TestRecordAgreementAcceptancesPreservesHistoryAndSourceMetadata(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreements([]AgreementDocument{
		{Key: "recharge_service", Version: "v1", Title: "Recharge", Content: "v1"},
		{Key: "recharge_service", Version: "v2", Title: "Recharge", Content: "v2"},
	})

	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{Key: "recharge_service", Version: "v1", Title: "Recharge", Content: "v1"},
	}, AgreementAcceptanceSource{ClientVersion: "desktop-1.0.0", RemoteAddr: "127.0.0.1:1234", DeviceSummary: "PinchBotTest/1.0"}); err != nil {
		t.Fatalf("RecordAgreementAcceptances(v1) error = %v", err)
	}
	if err := svc.RecordAgreementAcceptances(context.Background(), "user-1", []AgreementDocument{
		{Key: "recharge_service", Version: "v2", Title: "Recharge", Content: "v2"},
	}, AgreementAcceptanceSource{ClientVersion: "desktop-1.1.0", RemoteAddr: "127.0.0.1:4321", DeviceSummary: "PinchBotTest/1.1"}); err != nil {
		t.Fatalf("RecordAgreementAcceptances(v2) error = %v", err)
	}

	items, err := store.ListAgreementAcceptances(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListAgreementAcceptances() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("acceptance history len = %d, want 2", len(items))
	}
	if items[0].ClientVersion == "" || items[0].RemoteAddr == "" || items[0].DeviceSummary == "" {
		t.Fatalf("latest acceptance metadata = %#v, want populated source fields", items[0])
	}
}

func TestRefundRequestLifecycle(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 1200,
		Channel:   "manual",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}

	refund, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "unused balance")
	if err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	if refund.Status != "pending" {
		t.Fatalf("refund status = %q, want pending", refund.Status)
	}

	refund, err = svc.ReviewRefundRequest(context.Background(), refund.ID, RefundDecisionInput{
		Status:     "approved",
		ReviewedBy: "admin-1",
	})
	if err != nil {
		t.Fatalf("ReviewRefundRequest() error = %v", err)
	}
	if refund.Status != "refunded" {
		t.Fatalf("refund status = %q, want refunded", refund.Status)
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 1000 {
		t.Fatalf("balance = %d, want 1000 after refund", wallet.BalanceFen)
	}

	updatedOrder, err := svc.GetOrder(context.Background(), "user-1", order.ID)
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if updatedOrder.RefundedFen != 200 {
		t.Fatalf("refunded_fen = %d, want 200", updatedOrder.RefundedFen)
	}
	txs, err := svc.ListTransactions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListTransactions() error = %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("transaction count = %d, want 2 (recharge + refund)", len(txs))
	}
	if txs[len(txs)-1].Kind != "refund" && txs[0].Kind != "refund" {
		t.Fatalf("transactions = %#v, want one refund entry", txs)
	}
}

func TestGetOfficialAccessState(t *testing.T) {
	store := NewMemoryStore()
	store.SetBalance("user-1", 88)
	svc := NewService(store, nil)
	svc.SetOfficialModels([]OfficialModel{{ID: "official-basic", Name: "Official Basic", Enabled: true}})
	svc.SetPricingCatalog([]PricingRule{{ModelID: "official-basic", Version: "2026-03-13", FallbackPriceFen: 10}})

	state, err := svc.GetOfficialAccessState(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOfficialAccessState() error = %v", err)
	}
	if !state.Enabled {
		t.Fatal("expected official access to be enabled")
	}
	if !state.LowBalance {
		t.Fatal("expected balance to be reported as low")
	}
	if state.ModelsConfigured != 1 {
		t.Fatalf("models_configured = %d, want 1", state.ModelsConfigured)
	}
}

func TestApproveRefundRequestFallsBackToManualSettlementWhenProviderUnsupported(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	store.SetBalance("user-1", 1200)
	if err := store.SaveOrder(context.Background(), RechargeOrder{
		ID:          "ord-1",
		UserID:      "user-1",
		AmountFen:   1200,
		Status:      "paid",
		Provider:    "manual",
		CreatedUnix: 1,
		UpdatedUnix: 1,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.CreateRefundRequest(context.Background(), RefundRequest{
		ID:          "refund-1",
		UserID:      "user-1",
		OrderID:     "ord-1",
		AmountFen:   200,
		Status:      "pending",
		CreatedUnix: 1,
		UpdatedUnix: 1,
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}

	refund, err := svc.ApproveRefundRequest(context.Background(), "refund-1", RefundDecisionInput{ReviewedBy: "admin-1"})
	if err != nil {
		t.Fatalf("ApproveRefundRequest() error = %v", err)
	}
	if refund.Status != "approved_pending_payout" {
		t.Fatalf("refund status = %q, want approved_pending_payout", refund.Status)
	}

	refund, err = svc.MarkRefundSettled(context.Background(), "refund-1", RefundDecisionInput{ReviewedBy: "admin-1"})
	if err != nil {
		t.Fatalf("MarkRefundSettled() error = %v", err)
	}
	if refund.Status != "refunded" {
		t.Fatalf("refund status = %q, want refunded", refund.Status)
	}
}

func TestReconcileRechargeOrderFinalizesPendingOrderWhenProviderReportsPaid(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
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
	if err := store.SaveOrder(context.Background(), RechargeOrder{
		ID:          "ord-1",
		UserID:      "user-1",
		AmountFen:   300,
		Status:      "pending",
		Provider:    "easypay",
		CreatedUnix: 1,
		UpdatedUnix: 1,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}

	order, changed, err := svc.ReconcileRechargeOrder(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("ReconcileRechargeOrder() error = %v", err)
	}
	if !changed || order.Status != "paid" {
		t.Fatalf("order = %#v, changed=%t, want changed paid order", order, changed)
	}
}

type stubChatClient struct {
	response ChatResult
	err      error
}

func (s *stubChatClient) Chat(ctx context.Context, input ChatInput) (ChatResult, error) {
	return s.response, s.err
}

type stubOfficialProxyClient struct {
	response platformapi.ChatProxyResponse
	err      error
}

func (s stubOfficialProxyClient) ProxyChat(ctx context.Context, userID string, request platformapi.ChatProxyRequest) (platformapi.ChatProxyResponse, error) {
	return s.response, s.err
}

type recordingOfficialProxyClient struct {
	response platformapi.ChatProxyResponse
	err      error
	called   bool
}

func (s *recordingOfficialProxyClient) ProxyChat(ctx context.Context, userID string, request platformapi.ChatProxyRequest) (platformapi.ChatProxyResponse, error) {
	s.called = true
	return s.response, s.err
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

type partialRechargeStore struct {
	order     RechargeOrder
	creditErr error
}

func (s *partialRechargeStore) GetWallet(ctx context.Context, userID string) (WalletSummary, error) {
	return WalletSummary{UserID: userID, Currency: "CNY"}, nil
}
func (s *partialRechargeStore) SetBalance(userID string, balanceFen int64) {}
func (s *partialRechargeStore) AppendTransaction(ctx context.Context, tx WalletTransaction) error {
	return nil
}
func (s *partialRechargeStore) ListTransactions(ctx context.Context, userID string) ([]WalletTransaction, error) {
	return nil, nil
}
func (s *partialRechargeStore) SaveOrder(ctx context.Context, order RechargeOrder) error {
	s.order = order
	return nil
}
func (s *partialRechargeStore) GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error) {
	if s.order.ID != orderID || s.order.UserID != userID {
		return RechargeOrder{}, fmt.Errorf("order not found")
	}
	return s.order, nil
}
func (s *partialRechargeStore) MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (RechargeOrder, bool, error) {
	s.order.Status = "paid"
	s.order.Provider = provider
	s.order.ExternalID = externalID
	return s.order, true, nil
}
func (s *partialRechargeStore) FinalizeRechargeOrder(ctx context.Context, orderID, provider, externalID, description string) (RechargeOrder, WalletSummary, bool, error) {
	if s.creditErr != nil {
		return s.order, WalletSummary{}, false, s.creditErr
	}
	s.order.Status = "paid"
	s.order.Provider = provider
	s.order.ExternalID = externalID
	return s.order, WalletSummary{UserID: s.order.UserID, BalanceFen: s.order.AmountFen, Currency: "CNY"}, true, nil
}
func (s *partialRechargeStore) Credit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error) {
	return WalletSummary{}, s.creditErr
}
func (s *partialRechargeStore) Debit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error) {
	return WalletSummary{}, nil
}
func (s *partialRechargeStore) UpsertAdminEmails(ctx context.Context, emails []string) error {
	return nil
}
func (s *partialRechargeStore) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	return false, nil
}
func (s *partialRechargeStore) RecordAgreementAcceptance(ctx context.Context, acceptance AgreementAcceptance) error {
	return nil
}
func (s *partialRechargeStore) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	return false, nil
}
func (s *partialRechargeStore) FindOrderByID(ctx context.Context, orderID string) (RechargeOrder, error) {
	if s.order.ID != orderID {
		return RechargeOrder{}, fmt.Errorf("order not found")
	}
	return s.order, nil
}
func (s *partialRechargeStore) ListOrders(ctx context.Context) ([]RechargeOrder, error) {
	return nil, nil
}
func (s *partialRechargeStore) ListUsers(ctx context.Context) ([]UserSummary, error) {
	return nil, nil
}
func (s *partialRechargeStore) ListWalletAdjustments(ctx context.Context) ([]WalletTransaction, error) {
	return nil, nil
}
func (s *partialRechargeStore) AppendAuditLog(ctx context.Context, entry AdminAuditLog) error {
	return nil
}
func (s *partialRechargeStore) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AdminAuditLog, error) {
	return nil, nil
}
func (s *partialRechargeStore) ListAgreementAcceptances(ctx context.Context, userID string) ([]AgreementAcceptance, error) {
	return nil, nil
}
func (s *partialRechargeStore) RecordChatUsage(ctx context.Context, usage ChatUsageRecord) error {
	return nil
}
func (s *partialRechargeStore) CreateRefundRequest(ctx context.Context, request RefundRequest) error {
	return nil
}
func (s *partialRechargeStore) SaveRefundRequest(ctx context.Context, request RefundRequest) error {
	return nil
}
func (s *partialRechargeStore) GetRefundRequest(ctx context.Context, requestID string) (RefundRequest, error) {
	return RefundRequest{}, nil
}
func (s *partialRechargeStore) ListRefundRequests(ctx context.Context, userID string) ([]RefundRequest, error) {
	return nil, nil
}
func (s *partialRechargeStore) ApplyRefundDecision(ctx context.Context, requestID string, input RefundDecisionInput, updatedUnix int64) (RefundRequest, error) {
	return RefundRequest{}, nil
}
func (s *partialRechargeStore) CreateInfringementReport(ctx context.Context, report InfringementReport) error {
	return nil
}
func (s *partialRechargeStore) SaveInfringementReport(ctx context.Context, report InfringementReport) error {
	return nil
}
func (s *partialRechargeStore) GetInfringementReport(ctx context.Context, reportID string) (InfringementReport, error) {
	return InfringementReport{}, nil
}
func (s *partialRechargeStore) ListInfringementReports(ctx context.Context, userID string) ([]InfringementReport, error) {
	return nil, nil
}
func (s *partialRechargeStore) ListDataRetentionPolicies(ctx context.Context) ([]DataRetentionPolicy, error) {
	return nil, nil
}
func (s *partialRechargeStore) SaveDataRetentionPolicies(ctx context.Context, policies []DataRetentionPolicy) error {
	return nil
}
func (s *partialRechargeStore) ListSystemNotices(ctx context.Context) ([]SystemNotice, error) {
	return nil, nil
}
func (s *partialRechargeStore) SaveSystemNotices(ctx context.Context, notices []SystemNotice) error {
	return nil
}
func (s *partialRechargeStore) ListRiskRules(ctx context.Context) ([]RiskRule, error) {
	return nil, nil
}
func (s *partialRechargeStore) SaveRiskRules(ctx context.Context, rules []RiskRule) error {
	return nil
}
