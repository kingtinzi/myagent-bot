package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAliMPayCreateOrderBuildsSignedPayURL(t *testing.T) {
	provider := NewAliMPayProvider(AliMPayConfig{
		BaseURL:   "https://pay.example.com",
		PID:       "20001",
		Key:       "secret",
		NotifyURL: "https://platform.example.com/payments/alimpay/notify",
		ReturnURL: "https://platform.example.com/payments/alimpay/return",
		Type:      "alipay",
		SiteName:  "PinchBot",
	})

	order, err := provider.CreateOrder(context.Background(), CreateOrderInput{
		OrderID:   "ord_123",
		AmountFen: 1200,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.Provider != "alimpay" {
		t.Fatalf("provider = %q, want %q", order.Provider, "alimpay")
	}
	if !strings.HasPrefix(order.PayURL, "https://pay.example.com/submit.php?") {
		t.Fatalf("pay_url = %q, want submit.php URL", order.PayURL)
	}
	parsed, err := url.Parse(order.PayURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsed.Query()
	if query.Get("pid") != "20001" {
		t.Fatalf("pid = %q, want %q", query.Get("pid"), "20001")
	}
	if query.Get("out_trade_no") != "ord_123" {
		t.Fatalf("out_trade_no = %q, want %q", query.Get("out_trade_no"), "ord_123")
	}
	if query.Get("money") != "12.00" {
		t.Fatalf("money = %q, want %q", query.Get("money"), "12.00")
	}
	if query.Get("sitename") != "PinchBot" {
		t.Fatalf("sitename = %q, want %q", query.Get("sitename"), "PinchBot")
	}
	if query.Get("name") != "PinchBot Recharge" {
		t.Fatalf("name = %q, want %q", query.Get("name"), "PinchBot Recharge")
	}
	if query.Get("sign") == "" {
		t.Fatal("expected sign query param to be populated")
	}
}

func TestAliMPayVerifyCallbackAcceptsValidSignature(t *testing.T) {
	provider := NewAliMPayProvider(AliMPayConfig{
		BaseURL: "https://pay.example.com",
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	})

	values := url.Values{
		"pid":          []string{"20001"},
		"out_trade_no": []string{"ord_123"},
		"trade_no":     []string{"trade_456"},
		"type":         []string{"alipay"},
		"name":         []string{"PinchBot Recharge"},
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

func TestAliMPayQueryOrderParsesPaidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("action"); got != "order" {
			t.Fatalf("action = %q, want order", got)
		}
		if got := r.Form.Get("out_trade_no"); got != "ord_123" {
			t.Fatalf("out_trade_no = %q, want ord_123", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":         1,
			"msg":          "SUCCESS",
			"trade_no":     "trade_456",
			"out_trade_no": "ord_123",
			"money":        "12.34",
			"status":       1,
		})
	}))
	defer server.Close()

	provider := NewAliMPayProvider(AliMPayConfig{
		BaseURL: server.URL,
		PID:     "20001",
		Key:     "secret",
		Type:    "alipay",
	})

	result, err := provider.QueryOrder(context.Background(), QueryOrderInput{
		OrderID: "ord_123",
	})
	if err != nil {
		t.Fatalf("QueryOrder() error = %v", err)
	}
	if !result.Paid {
		t.Fatal("expected order to be marked paid")
	}
	if result.Status != "paid" {
		t.Fatalf("status = %q, want paid", result.Status)
	}
	if result.AmountFen != 1234 {
		t.Fatalf("amount_fen = %d, want 1234", result.AmountFen)
	}
	if result.ExternalOrderID != "trade_456" {
		t.Fatalf("external_order_id = %q, want trade_456", result.ExternalOrderID)
	}
}
