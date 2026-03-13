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

func TestEasyPayQueryOrderParsesPaidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("act"); got != "order" {
			t.Fatalf("act = %q, want order", got)
		}
		if got := r.Form.Get("out_trade_no"); got != "ord_123" {
			t.Fatalf("out_trade_no = %q, want ord_123", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1,
			"msg":  "ok",
			"data": map[string]any{
				"out_trade_no": "ord_123",
				"trade_no":     "trade_456",
				"money":        "12.34",
				"trade_status": "TRADE_SUCCESS",
				"status":       "1",
			},
		})
	}))
	defer server.Close()

	provider := NewEasyPayProvider(EasyPayConfig{
		BaseURL: server.URL,
		PID:     "10001",
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

func TestEasyPayRefundParsesSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("act"); got != "refund" {
			t.Fatalf("act = %q, want refund", got)
		}
		if got := r.Form.Get("trade_no"); got != "trade_456" {
			t.Fatalf("trade_no = %q, want trade_456", got)
		}
		if got := r.Form.Get("money"); got != "2.00" {
			t.Fatalf("money = %q, want 2.00", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1,
			"msg":  "ok",
			"data": map[string]any{
				"out_trade_no": "ord_123",
				"trade_no":     "trade_456",
				"refund_no":    "refund_789",
				"money":        "2.00",
				"status":       "success",
			},
		})
	}))
	defer server.Close()

	provider := NewEasyPayProvider(EasyPayConfig{
		BaseURL: server.URL,
		PID:     "10001",
		Key:     "secret",
		Type:    "alipay",
	})

	result, err := provider.Refund(context.Background(), RefundInput{
		OrderID:         "ord_123",
		ExternalOrderID: "trade_456",
		AmountFen:       200,
		Reason:          "user requested refund",
	})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected refund to succeed")
	}
	if result.ExternalRefundID != "refund_789" {
		t.Fatalf("external_refund_id = %q, want refund_789", result.ExternalRefundID)
	}
	if result.AmountFen != 200 {
		t.Fatalf("amount_fen = %d, want 200", result.AmountFen)
	}
}
