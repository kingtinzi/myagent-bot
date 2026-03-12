package payments

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestEasyPayCreateOrderBuildsSignedPayURL(t *testing.T) {
	provider := NewEasyPayProvider(EasyPayConfig{
		BaseURL:   "https://pay.example.com",
		PID:       "10001",
		Key:       "secret",
		NotifyURL: "https://platform.example.com/payments/easypay/notify",
		ReturnURL: "https://platform.example.com/payments/easypay/return",
		Type:      "alipay",
		SiteName:  "OpenClaw",
	})

	order, err := provider.CreateOrder(context.Background(), CreateOrderInput{
		OrderID:   "ord_123",
		AmountFen: 1200,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.Provider != "easypay" {
		t.Fatalf("provider = %q, want %q", order.Provider, "easypay")
	}
	if !strings.HasPrefix(order.PayURL, "https://pay.example.com/submit.php?") {
		t.Fatalf("pay_url = %q, want submit.php URL", order.PayURL)
	}
	parsed, err := url.Parse(order.PayURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsed.Query()
	if query.Get("out_trade_no") != "ord_123" {
		t.Fatalf("out_trade_no = %q, want %q", query.Get("out_trade_no"), "ord_123")
	}
	if query.Get("money") != "12.00" {
		t.Fatalf("money = %q, want %q", query.Get("money"), "12.00")
	}
	if query.Get("sign") == "" {
		t.Fatal("expected sign query param to be populated")
	}
}

func TestEasyPayVerifyCallbackAcceptsValidSignature(t *testing.T) {
	provider := NewEasyPayProvider(EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	})

	values := url.Values{
		"pid":          []string{"10001"},
		"out_trade_no": []string{"ord_123"},
		"trade_no":     []string{"trade_456"},
		"type":         []string{"alipay"},
		"name":         []string{"OpenClaw Recharge"},
		"money":        []string{"12.00"},
		"trade_status": []string{"TRADE_SUCCESS"},
		"sign_type":    []string{"MD5"},
	}
	values.Set("sign", provider.SignForTest(values))

	result, err := provider.VerifyCallback(context.Background(), values)
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if !result.Paid {
		t.Fatal("expected callback to be treated as paid")
	}
	if result.OrderID != "ord_123" {
		t.Fatalf("order_id = %q, want %q", result.OrderID, "ord_123")
	}
	if result.AmountFen != 1200 {
		t.Fatalf("amount_fen = %d, want %d", result.AmountFen, 1200)
	}
}

func TestEasyPayVerifyCallbackRejectsInvalidMoneyValue(t *testing.T) {
	provider := NewEasyPayProvider(EasyPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	})

	values := url.Values{
		"pid":          []string{"10001"},
		"out_trade_no": []string{"ord_123"},
		"trade_no":     []string{"trade_456"},
		"type":         []string{"alipay"},
		"name":         []string{"OpenClaw Recharge"},
		"money":        []string{"twelve"},
		"trade_status": []string{"TRADE_SUCCESS"},
		"sign_type":    []string{"MD5"},
	}
	values.Set("sign", provider.SignForTest(values))

	if _, err := provider.VerifyCallback(context.Background(), values); err == nil {
		t.Fatal("expected invalid money value to be rejected")
	}
}
