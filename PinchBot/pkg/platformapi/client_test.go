package platformapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("path = %q, want /auth/login", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(AuthResponse{
			Session: Session{AccessToken: "token-1", UserID: "user-1", Email: "user@example.com"},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	session, err := client.Login(context.Background(), AuthRequest{Email: "user@example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.AccessToken != "token-1" {
		t.Fatalf("access_token = %q, want token-1", session.AccessToken)
	}
}

func TestClientGetWallet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token-1")
		}
		_ = json.NewEncoder(w).Encode(WalletSummary{UserID: "user-1", BalanceFen: 1200, Currency: "CNY"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	wallet, err := client.GetWallet(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if wallet.BalanceFen != 1200 {
		t.Fatalf("balance = %d, want 1200", wallet.BalanceFen)
	}
}

func TestClientIncludesErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "agreement recharge_service version v1 must be accepted before recharge", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.CreateOrder(context.Background(), "token-1", CreateOrderRequest{AmountFen: 1200, Channel: "easypay"})
	if err == nil {
		t.Fatal("expected CreateOrder() to fail")
	}
	if !strings.Contains(err.Error(), "agreement recharge_service version v1") {
		t.Fatalf("error = %q, want body text included", err.Error())
	}
}

func TestClientReturnsAPIErrorStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetWallet(context.Background(), "token-1")
	if err == nil {
		t.Fatal("expected GetWallet() to fail")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
}
