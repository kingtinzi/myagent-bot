package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/service"
)

func TestAliMPayNotifyCreditsWallet(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SetPaymentProvider(payments.NewAliMPayProvider(payments.AliMPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	})); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-1", service.CreateOrderInput{
		AmountFen: 1200,
		Channel:   "alimpay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-1"}, nil, nil)

	values := make(url.Values)
	values.Set("pid", "20001")
	values.Set("out_trade_no", order.ID)
	values.Set("trade_no", "trade_456")
	values.Set("type", "alipay")
	values.Set("name", "PinchBot Recharge")
	values.Set("money", "12.00")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("sign_type", "MD5")
	values.Set("sign", payments.NewAliMPayProvider(payments.AliMPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	}).SignForTest(values))

	req := httptest.NewRequest(http.MethodPost, "/payments/alimpay/notify", strings.NewReader(values.Encode()))
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

func TestAliMPayNotifyCreditsWalletFromGETCallback(t *testing.T) {
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	if err := svc.SetPaymentProvider(payments.NewAliMPayProvider(payments.AliMPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	})); err != nil {
		t.Fatalf("SetPaymentProvider() error = %v", err)
	}
	order, err := svc.CreateRechargeOrder(context.Background(), "user-2", service.CreateOrderInput{
		AmountFen: 8800,
		Channel:   "alimpay",
	})
	if err != nil {
		t.Fatalf("CreateRechargeOrder() error = %v", err)
	}
	server := NewServer(svc, stubVerifier{userID: "user-2"}, nil, nil)

	values := make(url.Values)
	values.Set("pid", "20001")
	values.Set("out_trade_no", order.ID)
	values.Set("trade_no", "trade_789")
	values.Set("type", "alipay")
	values.Set("name", "PinchBot Recharge")
	values.Set("money", "88.00")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("sign_type", "MD5")
	values.Set("sign", payments.NewAliMPayProvider(payments.AliMPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	}).SignForTest(values))

	req := httptest.NewRequest(http.MethodGet, "/payments/alimpay/notify?"+values.Encode(), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	wallet, err := svc.GetWallet(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 8800 {
		t.Fatalf("balance = %d, want 8800", wallet.BalanceFen)
	}
}
