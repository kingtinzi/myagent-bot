package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sipeed/picoclaw/pkg/platformapi"
	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
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
	}); err != nil {
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
	})
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
	})
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
	}); err != nil {
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
func (s *partialRechargeStore) RecordAgreementAcceptance(ctx context.Context, userID, key, version string) error {
	return nil
}
func (s *partialRechargeStore) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	return false, nil
}
