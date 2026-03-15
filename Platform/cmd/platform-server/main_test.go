package main

import (
	"context"
	"net/url"
	"testing"

	"openclaw/platform/internal/config"
	"openclaw/platform/internal/payments"
)

func TestBuildPaymentProviderUsesPinchBotBrandingForEasyPay(t *testing.T) {
	provider := buildPaymentProvider(config.Config{
		PaymentProvider: "easypay",
		PublicBaseURL:   "https://platform.example.com",
		EasyPayBaseURL:  "https://pay.example.com",
		EasyPayPID:      "10001",
		EasyPayKey:      "secret",
		EasyPayType:     "alipay",
	})

	order, err := provider.CreateOrder(context.Background(), payments.CreateOrderInput{
		OrderID:   "ord_easy",
		AmountFen: 100,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	query := mustParseQuery(t, order.PayURL)
	if query.Get("sitename") != "PinchBot" {
		t.Fatalf("sitename = %q, want %q", query.Get("sitename"), "PinchBot")
	}
	if query.Get("name") != "PinchBot Recharge" {
		t.Fatalf("name = %q, want %q", query.Get("name"), "PinchBot Recharge")
	}
}

func TestBuildPaymentProviderUsesPinchBotBrandingForAliMPay(t *testing.T) {
	provider := buildPaymentProvider(config.Config{
		PaymentProvider: "alimpay",
		PublicBaseURL:   "https://platform.example.com",
		AliMPayBaseURL:  "https://pay.example.com",
		AliMPayPID:      "20001",
		AliMPayKey:      "secret",
		AliMPayType:     "alipay",
	})

	order, err := provider.CreateOrder(context.Background(), payments.CreateOrderInput{
		OrderID:   "ord_ali",
		AmountFen: 100,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	query := mustParseQuery(t, order.PayURL)
	if query.Get("sitename") != "PinchBot" {
		t.Fatalf("sitename = %q, want %q", query.Get("sitename"), "PinchBot")
	}
	if query.Get("name") != "PinchBot Recharge" {
		t.Fatalf("name = %q, want %q", query.Get("name"), "PinchBot Recharge")
	}
}

func mustParseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed.Query()
}
