package payments

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

var ErrOperationNotSupported = errors.New("payment provider operation not supported")

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

type QueryOrderInput struct {
	OrderID         string
	ExternalOrderID string
}

type OrderStatusResult struct {
	OrderID         string
	ExternalOrderID string
	AmountFen       int64
	Status          string
	ProviderStatus  string
	Paid            bool
	Refunded        bool
	LastCheckedUnix int64
}

type RefundInput struct {
	OrderID         string
	ExternalOrderID string
	AmountFen       int64
	Reason          string
}

type RefundResult struct {
	OrderID          string
	ExternalOrderID  string
	ExternalRefundID string
	AmountFen        int64
	Status           string
	ProviderStatus   string
	Succeeded        bool
	Pending          bool
	Message          string
}

type ProviderCapabilities struct {
	CanQueryOrder bool `json:"can_query_order"`
	CanRefund     bool `json:"can_refund"`
}

type Provider interface {
	CreateOrder(ctx context.Context, input CreateOrderInput) (PaymentOrder, error)
	VerifyCallback(ctx context.Context, values url.Values) (CallbackResult, error)
	QueryOrder(ctx context.Context, input QueryOrderInput) (OrderStatusResult, error)
	Refund(ctx context.Context, input RefundInput) (RefundResult, error)
	Capabilities() ProviderCapabilities
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

func (ManualProvider) QueryOrder(ctx context.Context, input QueryOrderInput) (OrderStatusResult, error) {
	return OrderStatusResult{}, ErrOperationNotSupported
}

func (ManualProvider) Refund(ctx context.Context, input RefundInput) (RefundResult, error) {
	return RefundResult{}, ErrOperationNotSupported
}

func (ManualProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{}
}
