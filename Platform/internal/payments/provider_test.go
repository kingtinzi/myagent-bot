package payments

import (
	"context"
	"testing"
)

func TestManualProviderCreateOrder(t *testing.T) {
	provider := ManualProvider{}
	order, err := provider.CreateOrder(context.Background(), CreateOrderInput{
		OrderID:   "ord_1",
		AmountFen: 1000,
		ReturnURL: "http://localhost/ok",
		NotifyURL: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.Status != "pending" {
		t.Fatalf("status = %q, want pending", order.Status)
	}
}
