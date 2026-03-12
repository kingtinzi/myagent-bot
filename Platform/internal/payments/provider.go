package payments

import (
	"context"
	"fmt"
	"net/url"
)

type CreateOrderInput struct {
	OrderID   string
	AmountFen int64
	ReturnURL string
	NotifyURL string
}

type PaymentOrder struct {
	OrderID         string `json:"order_id"`
	ExternalOrderID string `json:"external_order_id,omitempty"`
	Status          string `json:"status"`
	PayURL          string `json:"pay_url,omitempty"`
	Provider        string `json:"provider,omitempty"`
	AmountFen       int64  `json:"amount_fen"`
}

type CallbackResult struct {
	OrderID         string
	ExternalOrderID string
	AmountFen       int64
	Paid            bool
	Status          string
}

type Provider interface {
	CreateOrder(ctx context.Context, input CreateOrderInput) (PaymentOrder, error)
	VerifyCallback(ctx context.Context, values url.Values) (CallbackResult, error)
	Name() string
}

type ManualProvider struct{}

func (ManualProvider) Name() string { return "manual" }

func (ManualProvider) CreateOrder(ctx context.Context, input CreateOrderInput) (PaymentOrder, error) {
	return PaymentOrder{
		OrderID:   input.OrderID,
		Status:    "pending",
		Provider:  "manual",
		AmountFen: input.AmountFen,
	}, nil
}

func (ManualProvider) VerifyCallback(ctx context.Context, values url.Values) (CallbackResult, error) {
	return CallbackResult{}, fmt.Errorf("manual provider does not support async callbacks")
}
