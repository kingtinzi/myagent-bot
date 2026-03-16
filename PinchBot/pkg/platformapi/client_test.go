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

func TestClientSignUpIncludesAgreements(t *testing.T) {
	var got AuthRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/signup" {
			t.Fatalf("path = %q, want /auth/signup", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(AuthResponse{
			Session: Session{AccessToken: "token-1", UserID: "user-1", Email: "user@example.com"},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.SignUp(context.Background(), AuthRequest{
		Email:    "user@example.com",
		Password: "secret",
		Agreements: []AgreementDocument{
			{Key: "user_terms", Version: "v1", Title: "用户协议"},
			{Key: "privacy_policy", Version: "v1", Title: "隐私政策"},
		},
	})
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	if len(got.Agreements) != 2 {
		t.Fatalf("agreements = %#v, want two forwarded signup agreements", got.Agreements)
	}
}

func TestClientSignUpResponsePreservesRecoveryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/signup" {
			t.Fatalf("path = %q, want /auth/signup", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(AuthResponse{
			Session:               Session{AccessToken: "token-1", UserID: "user-1", Email: "user@example.com"},
			AgreementSyncRequired: true,
			Warning:               "signup succeeded, but agreement sync must be retried before recharge",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.SignUpResponse(context.Background(), AuthRequest{Email: "user@example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("SignUpResponse() error = %v", err)
	}
	if !resp.AgreementSyncRequired || resp.Warning == "" {
		t.Fatalf("response = %#v, want recovery metadata preserved", resp)
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
		t.Fatalf("error = %q, want body text included for low-level client callers", err.Error())
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

func TestUserFacingErrorMessageReturnsLocalizedAuthGuidance(t *testing.T) {
	message := UserFacingErrorMessage(&APIError{
		StatusCode: http.StatusUnauthorized,
		Message:    "Invalid login credentials",
	})

	if message != "邮箱或密码错误" {
		t.Fatalf("UserFacingErrorMessage() = %q, want localized user-facing message", message)
	}
}
