package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

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

func TestSyncAdminUsersRevokesRemovedSeedEmailsButKeepsManualOperatorsActive(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, AdminOperator{
		Email:  "finance@example.com",
		Role:   AdminRoleFinance,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}
	if err := svc.SyncAdminUsers(context.Background(), []string{"new-admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() second call error = %v", err)
	}

	oldAdmin, err := svc.IsAdminUser(context.Background(), "user-1", "admin@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() old admin error = %v", err)
	}
	if oldAdmin {
		t.Fatal("expected removed seeded admin email to lose access")
	}

	newAdmin, err := svc.IsAdminUser(context.Background(), "user-2", "new-admin@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() new admin error = %v", err)
	}
	if !newAdmin {
		t.Fatal("expected new admin email to remain active")
	}

	manualAdmin, err := svc.IsAdminUser(context.Background(), "finance-1", "finance@example.com")
	if err != nil {
		t.Fatalf("IsAdminUser() manual admin error = %v", err)
	}
	if !manualAdmin {
		t.Fatal("expected manually managed admin operator to remain active after seed sync")
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

func TestValidateRequiredAuthAgreementsRejectsMissingSignupAgreement(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreements([]AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v2", Title: "隐私政策", Content: "privacy"},
		{Key: "recharge_service", Version: "v3", Title: "充值协议", Content: "recharge"},
	})

	_, err := svc.ValidateRequiredAuthAgreements(context.Background(), []AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
	})
	if err == nil {
		t.Fatal("expected missing privacy policy to be rejected")
	}
	if !errors.Is(err, ErrInvalidAgreement) {
		t.Fatalf("error = %v, want ErrInvalidAgreement", err)
	}
	if !strings.Contains(err.Error(), "privacy_policy") {
		t.Fatalf("error = %v, want missing privacy_policy guidance", err)
	}
}

func TestValidateRequiredAuthAgreementsRejectsContentDrift(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreements([]AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "privacy"},
	})

	_, err := svc.ValidateRequiredAuthAgreements(context.Background(), []AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms"},
		{Key: "privacy_policy", Version: "v1", Title: "隐私政策", Content: "tampered"},
	})
	if err == nil {
		t.Fatal("expected tampered signup agreement to be rejected")
	}
	if !errors.Is(err, ErrInvalidAgreement) {
		t.Fatalf("error = %v, want ErrInvalidAgreement", err)
	}
}

func TestValidateRequiredAuthAgreementsReturnsCanonicalPublishedDocuments(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	svc.SetAgreements([]AgreementDocument{
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms", URL: "https://example.com/terms"},
		{Key: "privacy_policy", Version: "v2", Title: "隐私政策", Content: "privacy", URL: "https://example.com/privacy"},
		{Key: "recharge_service", Version: "v3", Title: "充值协议", Content: "recharge"},
	})

	docs, err := svc.ValidateRequiredAuthAgreements(context.Background(), []AgreementDocument{
		{Key: "privacy_policy", Version: "v2", Title: "隐私政策", Content: "privacy", URL: "https://example.com/privacy"},
		{Key: "user_terms", Version: "v1", Title: "用户协议", Content: "terms", URL: "https://example.com/terms"},
	})
	if err != nil {
		t.Fatalf("ValidateRequiredAuthAgreements() error = %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %#v, want two canonical signup agreements", docs)
	}
	if docs[0].Key != "privacy_policy" || docs[1].Key != "user_terms" {
		t.Fatalf("docs = %#v, want canonical sorted current signup agreements", docs)
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

	refund, err = svc.MarkRefundSettled(context.Background(), refund.ID, RefundDecisionInput{
		ReviewedBy: "admin-1",
	})
	if err != nil {
		t.Fatalf("MarkRefundSettled() error = %v", err)
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

func TestReconcileRechargeOrderDoesNotDowngradePaidOrderWhenProviderReturnsPending(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SetPaymentProvider(stubPaymentProvider{
		queryResult: payments.OrderStatusResult{
			OrderID:         "ord-1",
			ExternalOrderID: "trade-1",
			Status:          "closed",
			ProviderStatus:  "0",
			Paid:            false,
			LastCheckedUnix: 20,
		},
	}); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), RechargeOrder{
		ID:              "ord-1",
		UserID:          "user-1",
		AmountFen:       300,
		Status:          "paid",
		Provider:        "alimpay",
		ExternalID:      "trade-1",
		ProviderStatus:  "TRADE_SUCCESS",
		PaidUnix:        10,
		LastCheckedUnix: 10,
		CreatedUnix:     1,
		UpdatedUnix:     10,
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}

	order, changed, err := svc.ReconcileRechargeOrder(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("ReconcileRechargeOrder() error = %v", err)
	}
	if changed {
		t.Fatalf("changed = %t, want false when preserving paid order", changed)
	}
	if order.Status != "paid" {
		t.Fatalf("order = %#v, want paid order preserved", order)
	}
	if order.ProviderStatus != "0" || order.LastCheckedUnix != 20 {
		t.Fatalf("order = %#v, want latest provider status metadata without status downgrade", order)
	}
}

func TestReconcileRechargeOrderSettlesWalletWhenProviderReportsRefunded(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SetPaymentProvider(stubPaymentProvider{
		queryResult: payments.OrderStatusResult{
			OrderID:         "ord-1",
			ExternalOrderID: "trade-1",
			Status:          "refunded",
			ProviderStatus:  "TRADE_REFUNDED",
			Paid:            true,
			Refunded:        true,
			LastCheckedUnix: 30,
		},
	}); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 300,
		Channel:   "alimpay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "alimpay", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}

	reconciled, changed, err := svc.ReconcileRechargeOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("ReconcileRechargeOrder() error = %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want refunded reconciliation to mutate ledger state")
	}
	if reconciled.Status != "refunded" || reconciled.RefundedFen != 300 {
		t.Fatalf("order = %#v, want fully refunded order", reconciled)
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 0 {
		t.Fatalf("balance = %d, want 0 after refund reconciliation", wallet.BalanceFen)
	}

	txs, err := svc.ListTransactions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListTransactions() error = %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("transactions = %#v, want recharge credit plus refund payout", txs)
	}
	hasRefund := false
	for _, tx := range txs {
		if tx.Kind == "refund" && tx.AmountFen == -300 {
			hasRefund = true
			break
		}
	}
	if !hasRefund {
		t.Fatalf("transactions = %#v, want one refund payout transaction", txs)
	}
}

func TestReconcileRechargeOrderAllowsNegativeWalletForExternalRefundRecovery(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SetPaymentProvider(stubPaymentProvider{
		queryResult: payments.OrderStatusResult{
			OrderID:         "ord-1",
			ExternalOrderID: "trade-1",
			Status:          "refunded",
			ProviderStatus:  "TRADE_REFUNDED",
			Paid:            true,
			Refunded:        true,
			LastCheckedUnix: 30,
		},
	}); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 300,
		Channel:   "alimpay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "alimpay", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	if _, err := store.Debit(context.Background(), "user-1", 300, "spent after recharge"); err != nil {
		t.Fatalf("Debit() error = %v", err)
	}

	reconciled, changed, err := svc.ReconcileRechargeOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("ReconcileRechargeOrder() error = %v", err)
	}
	if !changed || reconciled.Status != "refunded" {
		t.Fatalf("reconciled = %#v changed=%t, want refunded order after recovery", reconciled, changed)
	}

	wallet, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != -300 {
		t.Fatalf("balance = %d, want -300 after external refund recovery", wallet.BalanceFen)
	}
}

func TestListOrdersAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	for _, order := range []RechargeOrder{
		{ID: "ord-1", UserID: "user-1", Status: "pending", Provider: "easypay", AmountFen: 100, CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "ord-2", UserID: "user-1", Status: "paid", Provider: "manual", AmountFen: 200, CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "ord-3", UserID: "user-2", Status: "paid", Provider: "easypay", AmountFen: 300, CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder(%s) error = %v", order.ID, err)
		}
	}

	items, err := svc.ListOrders(context.Background(), RechargeOrderFilter{
		UserID:   "user-1",
		Status:   "paid",
		Provider: "manual",
	})
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "ord-2" {
		t.Fatalf("items = %#v, want only ord-2", items)
	}

	items, err = svc.ListOrders(context.Background(), RechargeOrderFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListOrders() paged error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "ord-2" {
		t.Fatalf("paged items = %#v, want only ord-2", items)
	}
}

func TestListRefundRequestsAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	for _, request := range []RefundRequest{
		{ID: "refund-1", UserID: "user-1", OrderID: "ord-1", Status: "pending", AmountFen: 100, CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "refund-2", UserID: "user-1", OrderID: "ord-2", Status: "refunded", AmountFen: 200, CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "refund-3", UserID: "user-2", OrderID: "ord-3", Status: "approved_pending_payout", AmountFen: 300, CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.CreateRefundRequest(context.Background(), request); err != nil {
			t.Fatalf("CreateRefundRequest(%s) error = %v", request.ID, err)
		}
	}

	items, err := svc.ListRefundRequests(context.Background(), RefundRequestFilter{
		UserID:  "user-1",
		OrderID: "ord-2",
		Status:  "refunded",
	})
	if err != nil {
		t.Fatalf("ListRefundRequests() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "refund-2" {
		t.Fatalf("items = %#v, want only refund-2", items)
	}

	items, err = svc.ListRefundRequests(context.Background(), RefundRequestFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListRefundRequests() paged error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "refund-2" {
		t.Fatalf("paged items = %#v, want only refund-2", items)
	}
}

func TestListUsersAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	store.SetBalance("user-1", 100)
	store.SetBalance("user-2", 200)

	items, err := svc.ListUsers(context.Background(), UserSummaryFilter{UserID: "user-2"})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-2" {
		t.Fatalf("items = %#v, want only user-2", items)
	}

	paged := filterUserSummaries([]UserSummary{
		{UserID: "user-2", UpdatedUnix: 20},
		{UserID: "user-1", UpdatedUnix: 10},
	}, UserSummaryFilter{Limit: 1, Offset: 1})
	if len(paged) != 1 || paged[0].UserID != "user-1" {
		t.Fatalf("paged items = %#v, want only user-1", paged)
	}
}

func TestListUsersIncludesTrackedUserWithoutWallet(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	if err := svc.UpsertUserIdentity(context.Background(), UserIdentity{
		UserID: "user-9",
		Email:  "newuser@example.com",
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}

	items, err := svc.ListUsers(context.Background(), UserSummaryFilter{UserID: "user-9"})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-9" {
		t.Fatalf("items = %#v, want tracked user without wallet activity", items)
	}
}

func TestListUsersIncludesMirroredEmail(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	if err := svc.UpsertUserIdentity(context.Background(), UserIdentity{
		UserID: "user-9",
		Email:  "newuser@example.com",
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}

	items, err := svc.ListUsers(context.Background(), UserSummaryFilter{UserID: "user-9"})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(items) != 1 || items[0].Email != "newuser@example.com" {
		t.Fatalf("items = %#v, want mirrored email in admin user summary", items)
	}
}

func TestListUsersAppliesEmailFilterAndProfileTimestamps(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	if err := svc.UpsertUserIdentity(context.Background(), UserIdentity{
		UserID:       "user-9",
		Email:        "newuser@example.com",
		CreatedUnix:  100,
		UpdatedUnix:  150,
		LastSeenUnix: 200,
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := svc.UpsertUserIdentity(context.Background(), UserIdentity{
		UserID: "user-10",
		Email:  "other@example.com",
	}); err != nil {
		t.Fatalf("UpsertUserIdentity(other) error = %v", err)
	}

	items, err := svc.ListUsers(context.Background(), UserSummaryFilter{Email: "newuser@example.com"})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-9" {
		t.Fatalf("items = %#v, want only filtered user", items)
	}
	if items[0].CreatedUnix != 100 || items[0].LastSeenUnix != 200 {
		t.Fatalf("item = %#v, want created/last seen timestamps", items[0])
	}
}

func TestApplyAdminWalletAdjustmentCreatesTaggedTransaction(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	wallet, replayed, err := svc.ApplyAdminWalletAdjustment(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminWalletAdjustmentInput{
		UserID:      "user-1",
		AmountFen:   300,
		Description: "manual top-up",
		RequestID:   "adj-1",
	})
	if err != nil {
		t.Fatalf("ApplyAdminWalletAdjustment() error = %v", err)
	}
	if replayed {
		t.Fatal("first wallet adjustment unexpectedly marked as replayed")
	}
	if wallet.BalanceFen != 300 {
		t.Fatalf("balance = %d, want 300", wallet.BalanceFen)
	}

	txs, err := svc.ListWalletAdjustments(context.Background(), WalletAdjustmentFilter{
		UserID:        "user-1",
		ReferenceType: "admin_adjustment",
	})
	if err != nil {
		t.Fatalf("ListWalletAdjustments() error = %v", err)
	}
	if len(txs) != 1 || txs[0].ReferenceType != "admin_adjustment" {
		t.Fatalf("transactions = %#v, want one admin adjustment transaction", txs)
	}

	logs, err := svc.ListAuditLogs(context.Background(), AuditLogFilter{Action: "admin.wallet_adjustment.created"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit logs = %#v, want one admin wallet adjustment audit log", logs)
	}
}

func TestApplyAdminWalletAdjustmentRequiresRequestID(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	_, _, err := svc.ApplyAdminWalletAdjustment(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminWalletAdjustmentInput{
		UserID:    "user-1",
		AmountFen: 100,
	})
	if !errors.Is(err, ErrInvalidRequestID) {
		t.Fatalf("error = %v, want ErrInvalidRequestID", err)
	}
}

func TestApplyAdminManualRechargeCreatesTaggedTransaction(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	wallet, replayed, err := svc.ApplyAdminManualRecharge(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminManualRechargeInput{
		UserID:      "user-1",
		AmountFen:   500,
		Description: "admin grant",
		RequestID:   "recharge-1",
	})
	if err != nil {
		t.Fatalf("ApplyAdminManualRecharge() error = %v", err)
	}
	if replayed {
		t.Fatal("first manual recharge unexpectedly marked as replayed")
	}
	if wallet.BalanceFen != 500 {
		t.Fatalf("balance = %d, want 500", wallet.BalanceFen)
	}

	txs, err := svc.ListWalletAdjustments(context.Background(), WalletAdjustmentFilter{
		UserID:        "user-1",
		ReferenceType: "admin_manual_recharge",
	})
	if err != nil {
		t.Fatalf("ListWalletAdjustments() error = %v", err)
	}
	if len(txs) != 1 || txs[0].ReferenceType != "admin_manual_recharge" || txs[0].Kind != "credit" {
		t.Fatalf("transactions = %#v, want one admin manual recharge credit transaction", txs)
	}

	logs, err := svc.ListAuditLogs(context.Background(), AuditLogFilter{Action: "admin.manual_recharge.created"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit logs = %#v, want one admin manual recharge audit log", logs)
	}
}

func TestApplyAdminManualRechargeRequiresRequestID(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	_, _, err := svc.ApplyAdminManualRecharge(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminManualRechargeInput{
		UserID:    "user-1",
		AmountFen: 100,
	})
	if !errors.Is(err, ErrInvalidRequestID) {
		t.Fatalf("error = %v, want ErrInvalidRequestID", err)
	}
}

func TestApplyAdminManualRechargeIsIdempotentByRequestID(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	firstWallet, replayed, err := svc.ApplyAdminManualRecharge(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminManualRechargeInput{
		UserID:      "user-1",
		AmountFen:   500,
		Description: "admin grant",
		RequestID:   "recharge-1",
	})
	if err != nil {
		t.Fatalf("first ApplyAdminManualRecharge() error = %v", err)
	}
	if replayed {
		t.Fatal("first recharge unexpectedly marked as replayed")
	}
	secondWallet, replayed, err := svc.ApplyAdminManualRecharge(context.Background(), AdminActor{
		UserID: "admin-1",
		Email:  "admin@example.com",
	}, AdminManualRechargeInput{
		UserID:      "user-1",
		AmountFen:   500,
		Description: "admin grant",
		RequestID:   "recharge-1",
	})
	if err != nil {
		t.Fatalf("second ApplyAdminManualRecharge() error = %v", err)
	}
	if !replayed {
		t.Fatal("expected second recharge to be treated as replay")
	}
	if firstWallet.BalanceFen != 500 || secondWallet.BalanceFen != 500 {
		t.Fatalf("wallets = %#v / %#v, want balance 500 for idempotent replay", firstWallet, secondWallet)
	}

	txs, err := svc.ListWalletAdjustments(context.Background(), WalletAdjustmentFilter{
		UserID:        "user-1",
		ReferenceType: "admin_manual_recharge",
	})
	if err != nil {
		t.Fatalf("ListWalletAdjustments() error = %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("transactions = %#v, want one idempotent manual recharge record", txs)
	}
	logs, err := svc.ListAuditLogs(context.Background(), AuditLogFilter{Action: "admin.manual_recharge.created"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit logs = %#v, want one idempotent manual recharge audit log", logs)
	}
}

func TestApplyAdminManualRechargeRequiresPositiveAmount(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	for _, amount := range []int64{0, -1} {
		_, _, err := svc.ApplyAdminManualRecharge(context.Background(), AdminActor{
			UserID: "admin-1",
			Email:  "admin@example.com",
		}, AdminManualRechargeInput{
			UserID:    "user-1",
			AmountFen: amount,
			RequestID: fmt.Sprintf("recharge-%d", amount),
		})
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("amount %d error = %v, want ErrInvalidAmount", amount, err)
		}
	}
}

func TestListWalletAdjustmentsAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	for _, tx := range []WalletTransaction{
		{ID: "tx-1", UserID: "user-1", Kind: "credit", AmountFen: 100, Description: "credit 1", CreatedUnix: 1},
		{ID: "tx-2", UserID: "user-2", Kind: "credit", AmountFen: 200, Description: "credit 2", CreatedUnix: 2},
		{ID: "tx-3", UserID: "user-2", Kind: "debit", AmountFen: -50, Description: "debit 2", CreatedUnix: 3},
	} {
		if err := store.AppendTransaction(context.Background(), tx); err != nil {
			t.Fatalf("AppendTransaction(%s) error = %v", tx.ID, err)
		}
	}

	items, err := svc.ListWalletAdjustments(context.Background(), WalletAdjustmentFilter{
		UserID: "user-2",
		Kind:   "debit",
	})
	if err != nil {
		t.Fatalf("ListWalletAdjustments() error = %v", err)
	}
	if len(items) != 1 || items[0].UserID != "user-2" || items[0].Kind != "debit" {
		t.Fatalf("items = %#v, want only user-2 debit", items)
	}

	items, err = svc.ListWalletAdjustments(context.Background(), WalletAdjustmentFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListWalletAdjustments() paged error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("paged items = %#v, want one item", items)
	}
}

func TestListInfringementReportsAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	for _, report := range []InfringementReport{
		{ID: "ipr-1", UserID: "user-1", Subject: "s1", Description: "d1", Status: "pending", CreatedUnix: 1, UpdatedUnix: 1},
		{ID: "ipr-2", UserID: "user-2", Subject: "s2", Description: "d2", Status: "resolved", ReviewedBy: "admin-1", CreatedUnix: 2, UpdatedUnix: 2},
		{ID: "ipr-3", UserID: "user-2", Subject: "s3", Description: "d3", Status: "reviewing", ReviewedBy: "admin-2", CreatedUnix: 3, UpdatedUnix: 3},
	} {
		if err := store.CreateInfringementReport(context.Background(), report); err != nil {
			t.Fatalf("CreateInfringementReport(%s) error = %v", report.ID, err)
		}
	}

	items, err := svc.ListInfringementReports(context.Background(), InfringementReportFilter{
		UserID:     "user-2",
		Status:     "resolved",
		ReviewedBy: "admin-1",
	})
	if err != nil {
		t.Fatalf("ListInfringementReports() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "ipr-2" {
		t.Fatalf("items = %#v, want only ipr-2", items)
	}

	items, err = svc.ListInfringementReports(context.Background(), InfringementReportFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListInfringementReports() paged error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "ipr-2" {
		t.Fatalf("paged items = %#v, want only ipr-2", items)
	}
}

func TestGetAdminOperatorReturnsDefaultRoleAndCapabilities(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}

	operator, err := svc.GetAdminOperator(context.Background(), "admin-1", "user@example.com")
	if err != nil {
		t.Fatalf("GetAdminOperator() error = %v", err)
	}
	if operator.Role != AdminRoleSuperAdmin {
		t.Fatalf("role = %q, want %q", operator.Role, AdminRoleSuperAdmin)
	}
	if !operator.Active {
		t.Fatalf("operator = %#v, want active admin operator", operator)
	}
	if !operator.HasCapability(AdminCapabilityDashboardRead) || !operator.HasCapability(AdminCapabilityWalletWrite) {
		t.Fatalf("operator capabilities = %#v, want default super-admin capabilities", operator.Capabilities)
	}
}

func TestGetAdminOperatorRejectsEmailRebindForDifferentUserID(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.GetAdminOperator(context.Background(), "admin-1", "user@example.com"); err != nil {
		t.Fatalf("initial GetAdminOperator() error = %v", err)
	}

	_, err := svc.GetAdminOperator(context.Background(), "admin-2", "user@example.com")
	if !errors.Is(err, ErrAdminAccessDenied) {
		t.Fatalf("error = %v, want ErrAdminAccessDenied after user binding mismatch", err)
	}
}

func TestGetAdminOperatorReturnsErrorWhenBindingPersistenceFails(t *testing.T) {
	store := &failingAdminOperatorStore{
		MemoryStore:           NewMemoryStore(),
		failSaveAdminOperator: true,
	}
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}

	_, err := svc.GetAdminOperator(context.Background(), "admin-1", "user@example.com")
	if err == nil {
		t.Fatal("expected GetAdminOperator() to fail when binding persistence fails")
	}
}

func TestRequireAdminCapabilityRejectsReadOnlyOperator(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	if err := svc.SyncAdminUsers(context.Background(), []string{"user@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}
	if _, err := svc.SaveAdminOperator(context.Background(), AdminActor{
		UserID: "admin-root",
		Email:  "root@example.com",
	}, AdminOperator{
		Email:  "user@example.com",
		Role:   AdminRoleReadOnly,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}

	_, err := svc.RequireAdminCapability(context.Background(), "admin-1", "user@example.com", AdminCapabilityWalletWrite)
	if !errors.Is(err, ErrAdminCapabilityDenied) {
		t.Fatalf("error = %v, want ErrAdminCapabilityDenied", err)
	}
}

func TestSaveAdminOperatorRejectsUnknownRole(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	_, err := svc.SaveAdminOperator(context.Background(), AdminActor{
		UserID: "admin-root",
		Email:  "root@example.com",
	}, AdminOperator{
		Email:  "user@example.com",
		Role:   "root-plus",
		Active: true,
	})
	if !errors.Is(err, ErrInvalidAdminRole) {
		t.Fatalf("error = %v, want ErrInvalidAdminRole", err)
	}
}

func TestListAdminOperatorsRepairsLegacyInactiveRows(t *testing.T) {
	store := NewMemoryStore()
	store.adminOperators["legacy@example.com"] = AdminOperator{
		Email:  "legacy@example.com",
		Active: false,
	}
	svc := NewService(store, nil)

	operators, err := svc.ListAdminOperators(context.Background())
	if err != nil {
		t.Fatalf("ListAdminOperators() error = %v", err)
	}
	if len(operators) != 1 {
		t.Fatalf("operators = %#v, want one repaired legacy operator", operators)
	}
	if operators[0].Role != AdminRoleSuperAdmin || len(operators[0].Capabilities) == 0 {
		t.Fatalf("operator = %#v, want repaired legacy role/capabilities", operators[0])
	}
}

func TestCreateInfringementReportRejectsUnsafeEvidenceURL(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	_, err := svc.CreateInfringementReport(context.Background(), InfringementReport{
		UserID:       "user-1",
		Subject:      "copyright issue",
		Description:  "unsafe evidence link",
		EvidenceURLs: []string{"javascript:alert(1)"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid evidence url") {
		t.Fatalf("error = %v, want invalid evidence url rejection", err)
	}
}

func TestReviewRefundRequestRejectsApprovedAlias(t *testing.T) {
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

	_, err = svc.ReviewRefundRequest(context.Background(), refund.ID, RefundDecisionInput{
		Status:     "approved",
		ReviewedBy: "admin-1",
	})
	if !errors.Is(err, ErrRefundNotAllowed) {
		t.Fatalf("error = %v, want ErrRefundNotAllowed", err)
	}
}

func TestRefundSettlementRejectsDuplicateRefundBeyondOrderAmount(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)

	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", CreateOrderInput{
		AmountFen: 300,
		Channel:   "manual",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	if _, err := svc.HandleSuccessfulRecharge(context.Background(), order.ID, "manual", "trade-1"); err != nil {
		t.Fatalf("HandleSuccessfulRecharge() error = %v", err)
	}
	store.SetBalance("user-1", 500)

	first, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "first refund")
	if err != nil {
		t.Fatalf("CreateRefundRequest(first) error = %v", err)
	}
	second, err := svc.CreateRefundRequest(context.Background(), "user-1", 200, order.ID, "second refund")
	if err != nil {
		t.Fatalf("CreateRefundRequest(second) error = %v", err)
	}
	if _, err := svc.MarkRefundSettled(context.Background(), first.ID, RefundDecisionInput{ReviewedBy: "admin-1"}); err != nil {
		t.Fatalf("MarkRefundSettled(first) error = %v", err)
	}

	_, err = svc.MarkRefundSettled(context.Background(), second.ID, RefundDecisionInput{ReviewedBy: "admin-1"})
	if !errors.Is(err, ErrRefundNotAllowed) {
		t.Fatalf("error = %v, want ErrRefundNotAllowed", err)
	}
}

func TestListChatUsageRecordsAppliesFiltersAndPagination(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	baseNow := time.Unix(20*24*3600, 0)
	for _, usage := range []ChatUsageRecord{
		{ID: "usage-1", UserID: "user-1", ModelID: "official-basic", ChargedFen: 12, CreatedUnix: baseNow.Add(-72 * time.Hour).Unix()},
		{ID: "usage-2", UserID: "user-2", ModelID: "official-pro", ChargedFen: 40, CreatedUnix: baseNow.Add(-48 * time.Hour).Unix()},
		{ID: "usage-3", UserID: "user-2", ModelID: "official-pro", ChargedFen: 20, CreatedUnix: baseNow.Add(-24 * time.Hour).Unix()},
	} {
		if err := store.RecordChatUsage(context.Background(), usage); err != nil {
			t.Fatalf("RecordChatUsage(%s) error = %v", usage.ID, err)
		}
	}

	items, err := svc.ListChatUsageRecords(context.Background(), ChatUsageRecordFilter{
		UserID:    "user-2",
		ModelID:   "official-pro",
		SinceUnix: baseNow.Add(-7 * 24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("ListChatUsageRecords() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != "usage-3" || items[1].ID != "usage-2" {
		t.Fatalf("items = %#v, want newest matching usage records", items)
	}

	paged, err := svc.ListChatUsageRecords(context.Background(), ChatUsageRecordFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListChatUsageRecords(paged) error = %v", err)
	}
	if len(paged) != 1 || paged[0].ID != "usage-2" {
		t.Fatalf("paged = %#v, want second newest usage record", paged)
	}
}

func TestGetAdminDashboardSummarizesRecentActivity(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	baseNow := time.Unix(30*24*3600, 0)
	svc.now = func() time.Time { return baseNow }

	store.SetBalance("user-1", 600)
	store.SetBalance("user-2", 900)
	for _, identity := range []UserIdentity{
		{UserID: "user-1", Email: "one@example.com", CreatedUnix: baseNow.Add(-10 * 24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-2 * time.Hour).Unix(), LastSeenUnix: baseNow.Add(-2 * time.Hour).Unix()},
		{UserID: "user-2", Email: "two@example.com", CreatedUnix: baseNow.Add(-2 * 24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-1 * time.Hour).Unix(), LastSeenUnix: baseNow.Add(-1 * time.Hour).Unix()},
	} {
		if err := svc.UpsertUserIdentity(context.Background(), identity); err != nil {
			t.Fatalf("UpsertUserIdentity(%s) error = %v", identity.UserID, err)
		}
	}
	for _, order := range []RechargeOrder{
		{ID: "ord-1", UserID: "user-1", Status: "paid", Provider: "manual", AmountFen: 500, CreatedUnix: baseNow.Add(-3 * 24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-3 * 24 * time.Hour).Unix()},
		{ID: "ord-2", UserID: "user-2", Status: "pending", Provider: "manual", AmountFen: 300, CreatedUnix: baseNow.Add(-24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-24 * time.Hour).Unix()},
	} {
		if err := store.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder(%s) error = %v", order.ID, err)
		}
	}
	if err := store.CreateRefundRequest(context.Background(), RefundRequest{
		ID: "refund-1", UserID: "user-1", OrderID: "ord-1", AmountFen: 120, Status: "pending",
		CreatedUnix: baseNow.Add(-12 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-12 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	for _, usage := range []ChatUsageRecord{
		{ID: "usage-1", UserID: "user-1", ModelID: "official-basic", ChargedFen: 30, PromptTokens: 100, CompletionTokens: 50, CreatedUnix: baseNow.Add(-2 * 24 * time.Hour).Unix()},
		{ID: "usage-2", UserID: "user-2", ModelID: "official-pro", ChargedFen: 80, PromptTokens: 200, CompletionTokens: 90, CreatedUnix: baseNow.Add(-18 * time.Hour).Unix()},
		{ID: "usage-3", UserID: "user-1", ModelID: "official-basic", ChargedFen: 10, PromptTokens: 50, CompletionTokens: 25, CreatedUnix: baseNow.Add(-9 * 24 * time.Hour).Unix()},
	} {
		if err := store.RecordChatUsage(context.Background(), usage); err != nil {
			t.Fatalf("RecordChatUsage(%s) error = %v", usage.ID, err)
		}
	}

	dashboard, err := svc.GetAdminDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetAdminDashboard() error = %v", err)
	}
	if dashboard.Totals.Users != 2 || dashboard.Totals.PaidOrders != 1 {
		t.Fatalf("totals = %#v, want users=2 paid_orders=1", dashboard.Totals)
	}
	if dashboard.Totals.WalletBalanceFen != 1500 || dashboard.Totals.RefundPending != 1 {
		t.Fatalf("totals = %#v, want wallet balance 1500 and one pending refund", dashboard.Totals)
	}
	if dashboard.Recent.RechargeFen7D != 500 || dashboard.Recent.ConsumptionFen7D != 110 || dashboard.Recent.NewUsers7D != 1 {
		t.Fatalf("recent = %#v, want recharge=500 consumption=110 new_users=1", dashboard.Recent)
	}
	if len(dashboard.TopModels) != 2 || dashboard.TopModels[0].ModelID != "official-pro" {
		t.Fatalf("top_models = %#v, want official-pro ranked first", dashboard.TopModels)
	}
}

func TestGetAdminDashboardExcludesActiveAdminTrafficFromFallbackSummary(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	baseNow := time.Unix(40*24*3600, 0)
	svc.now = func() time.Time { return baseNow }

	if _, err := svc.SaveAdminOperator(context.Background(), AdminActor{
		UserID: "root-1",
		Email:  "root@example.com",
	}, AdminOperator{
		UserID: "admin-1",
		Email:  "admin@example.com",
		Role:   AdminRoleSuperAdmin,
		Active: true,
	}); err != nil {
		t.Fatalf("SaveAdminOperator() error = %v", err)
	}

	for _, identity := range []UserIdentity{
		{UserID: "user-1", Email: "one@example.com", CreatedUnix: baseNow.Add(-2 * 24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-2 * time.Hour).Unix(), LastSeenUnix: baseNow.Add(-2 * time.Hour).Unix()},
		{UserID: "admin-1", Email: "admin@example.com", CreatedUnix: baseNow.Add(-24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-1 * time.Hour).Unix(), LastSeenUnix: baseNow.Add(-1 * time.Hour).Unix()},
	} {
		if err := svc.UpsertUserIdentity(context.Background(), identity); err != nil {
			t.Fatalf("UpsertUserIdentity(%s) error = %v", identity.UserID, err)
		}
	}
	store.SetBalance("user-1", 600)
	store.SetBalance("admin-1", 9999)
	for _, order := range []RechargeOrder{
		{ID: "ord-user", UserID: "user-1", Status: "paid", Provider: "manual", AmountFen: 500, CreatedUnix: baseNow.Add(-2 * 24 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-2 * 24 * time.Hour).Unix()},
		{ID: "ord-admin", UserID: "admin-1", Status: "paid", Provider: "manual", AmountFen: 999, CreatedUnix: baseNow.Add(-12 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-12 * time.Hour).Unix()},
	} {
		if err := store.SaveOrder(context.Background(), order); err != nil {
			t.Fatalf("SaveOrder(%s) error = %v", order.ID, err)
		}
	}
	for _, usage := range []ChatUsageRecord{
		{ID: "usage-user", UserID: "user-1", ModelID: "official-basic", ChargedFen: 30, PromptTokens: 100, CompletionTokens: 50, CreatedUnix: baseNow.Add(-6 * time.Hour).Unix()},
		{ID: "usage-admin", UserID: "admin-1", ModelID: "official-pro", ChargedFen: 999, PromptTokens: 200, CompletionTokens: 100, CreatedUnix: baseNow.Add(-3 * time.Hour).Unix()},
	} {
		if err := store.RecordChatUsage(context.Background(), usage); err != nil {
			t.Fatalf("RecordChatUsage(%s) error = %v", usage.ID, err)
		}
	}

	dashboard, err := svc.GetAdminDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetAdminDashboard() error = %v", err)
	}
	if dashboard.Totals.Users != 1 || dashboard.Totals.PaidOrders != 1 {
		t.Fatalf("totals = %#v, want only non-admin user/order counted", dashboard.Totals)
	}
	if dashboard.Totals.WalletBalanceFen != 600 {
		t.Fatalf("wallet balance = %d, want 600 without admin wallet", dashboard.Totals.WalletBalanceFen)
	}
	if dashboard.Recent.RechargeFen7D != 500 || dashboard.Recent.ConsumptionFen7D != 30 {
		t.Fatalf("recent = %#v, want only non-admin recharge and usage included", dashboard.Recent)
	}
	if len(dashboard.TopModels) != 1 || dashboard.TopModels[0].ModelID != "official-basic" {
		t.Fatalf("top_models = %#v, want admin usage excluded", dashboard.TopModels)
	}
}

func TestGetAdminDashboardUsesStoreBackedSummaryWhenAvailable(t *testing.T) {
	store := &dashboardSummaryStore{
		MemoryStore: NewMemoryStore(),
		dashboard: AdminDashboard{
			Totals: AdminDashboardTotals{
				Users:            7,
				PaidOrders:       3,
				WalletBalanceFen: 1234,
			},
			Recent: AdminDashboardRecent{
				RechargeFen7D:    900,
				ConsumptionFen7D: 321,
				NewUsers7D:       2,
			},
			TopModels: []AdminDashboardModelStat{{ModelID: "official-pro", UsageCount: 4, ChargedFen: 321}},
		},
	}
	svc := NewService(store, nil)
	baseNow := time.Unix(50*24*3600, 0)
	svc.now = func() time.Time { return baseNow }
	if err := svc.SyncAdminUsers(context.Background(), []string{"admin@example.com"}); err != nil {
		t.Fatalf("SyncAdminUsers() error = %v", err)
	}

	dashboard, err := svc.GetAdminDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetAdminDashboard() error = %v", err)
	}
	if !store.called {
		t.Fatal("expected store-backed dashboard summary path to be used")
	}
	if dashboard.GeneratedUnix != baseNow.Unix() {
		t.Fatalf("generated_unix = %d, want %d", dashboard.GeneratedUnix, baseNow.Unix())
	}
	if dashboard.Totals.Users != 7 || len(dashboard.TopModels) != 1 || dashboard.TopModels[0].ModelID != "official-pro" {
		t.Fatalf("dashboard = %#v, want store-provided dashboard summary", dashboard)
	}
}

func TestGetAdminUserOverviewIncludesRelatedData(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil)
	baseNow := time.Unix(40*24*3600, 0)
	svc.now = func() time.Time { return baseNow }

	store.SetBalance("user-2", 880)
	if err := svc.UpsertUserIdentity(context.Background(), UserIdentity{
		UserID:       "user-2",
		Email:        "detail@example.com",
		CreatedUnix:  baseNow.Add(-48 * time.Hour).Unix(),
		UpdatedUnix:  baseNow.Add(-6 * time.Hour).Unix(),
		LastSeenUnix: baseNow.Add(-6 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("UpsertUserIdentity() error = %v", err)
	}
	if err := store.SaveOrder(context.Background(), RechargeOrder{
		ID: "ord-detail", UserID: "user-2", Status: "paid", Provider: "manual", AmountFen: 500,
		CreatedUnix: baseNow.Add(-12 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-12 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("SaveOrder() error = %v", err)
	}
	if err := store.AppendTransaction(context.Background(), WalletTransaction{
		ID: "tx-detail", UserID: "user-2", Kind: "credit", AmountFen: 500, Description: "topup", CreatedUnix: baseNow.Add(-11 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("AppendTransaction() error = %v", err)
	}
	if err := store.RecordAgreementAcceptance(context.Background(), AgreementAcceptance{
		UserID: "user-2", AgreementKey: "recharge_service", Version: "v1", AcceptedUnix: baseNow.Add(-10 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("RecordAgreementAcceptance() error = %v", err)
	}
	if err := store.RecordChatUsage(context.Background(), ChatUsageRecord{
		ID: "usage-detail", UserID: "user-2", ModelID: "official-pro", ChargedFen: 66, CreatedUnix: baseNow.Add(-9 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("RecordChatUsage() error = %v", err)
	}
	if err := store.CreateRefundRequest(context.Background(), RefundRequest{
		ID: "refund-detail", UserID: "user-2", OrderID: "ord-detail", AmountFen: 120, Status: "pending",
		CreatedUnix: baseNow.Add(-8 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-8 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("CreateRefundRequest() error = %v", err)
	}
	if err := store.CreateInfringementReport(context.Background(), InfringementReport{
		ID: "ipr-detail", UserID: "user-2", Subject: "copyright", Description: "reported", Status: "pending",
		CreatedUnix: baseNow.Add(-7 * time.Hour).Unix(), UpdatedUnix: baseNow.Add(-7 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("CreateInfringementReport() error = %v", err)
	}

	overview, err := svc.GetAdminUserOverview(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("GetAdminUserOverview() error = %v", err)
	}
	if overview.User.UserID != "user-2" || overview.Wallet.BalanceFen != 880 {
		t.Fatalf("overview = %#v, want user profile + wallet balance", overview)
	}
	if len(overview.RecentOrders) != 1 || len(overview.RecentTransactions) != 1 || len(overview.Agreements) != 1 || len(overview.RecentUsage) != 1 {
		t.Fatalf("overview = %#v, want recent linked collections", overview)
	}
	if overview.PendingRefundCount != 1 || overview.PendingInfringementCount != 1 {
		t.Fatalf("overview = %#v, want pending governance counts", overview)
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

type dashboardSummaryStore struct {
	*MemoryStore
	dashboard AdminDashboard
	called    bool
}

func (s *dashboardSummaryStore) BuildAdminDashboard(ctx context.Context, input AdminDashboardStoreInput) (AdminDashboard, error) {
	s.called = true
	return s.dashboard, nil
}

type partialRechargeStore struct {
	order     RechargeOrder
	creditErr error
}

type failingAdminOperatorStore struct {
	*MemoryStore
	failSaveAdminOperator bool
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
func (s *partialRechargeStore) GetAdminOperator(ctx context.Context, userID, email string) (AdminOperator, error) {
	return AdminOperator{}, nil
}
func (s *partialRechargeStore) ListAdminOperators(ctx context.Context) ([]AdminOperator, error) {
	return nil, nil
}
func (s *partialRechargeStore) SaveAdminOperator(ctx context.Context, operator AdminOperator) (AdminOperator, error) {
	return operator, nil
}

func (s *failingAdminOperatorStore) SaveAdminOperator(ctx context.Context, operator AdminOperator) (AdminOperator, error) {
	if s.failSaveAdminOperator {
		return AdminOperator{}, errors.New("admin operator store unavailable")
	}
	return s.MemoryStore.SaveAdminOperator(ctx, operator)
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
func (s *partialRechargeStore) ListChatUsageRecords(ctx context.Context, filter ChatUsageRecordFilter) ([]ChatUsageRecord, error) {
	return nil, nil
}
func (s *partialRechargeStore) UpsertUserIdentity(ctx context.Context, identity UserIdentity) error {
	return nil
}
func (s *partialRechargeStore) ApplyWalletAdjustment(ctx context.Context, tx WalletTransaction) (WalletSummary, error) {
	return WalletSummary{}, nil
}
func (s *partialRechargeStore) ApplyAdminWalletMutation(ctx context.Context, tx WalletTransaction, audit AdminAuditLog) (WalletSummary, bool, error) {
	return WalletSummary{}, false, nil
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
