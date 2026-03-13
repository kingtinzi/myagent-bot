package authbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func TestLoginReturnsAPIErrorWithReadableMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"msg":"Invalid login credentials"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "anon-key")
	_, err := client.Login(context.Background(), platformapi.AuthRequest{
		Email:    "user@example.com",
		Password: "wrong",
	})
	if err == nil {
		t.Fatal("expected login to fail")
	}

	apiErr, ok := err.(*platformapi.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *platformapi.APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(apiErr.Message, "Invalid login credentials") {
		t.Fatalf("Message = %q, want readable upstream error", apiErr.Message)
	}
}

func TestSignUpFallsBackToLoginWhenSupabaseDoesNotReturnSession(t *testing.T) {
	t.Helper()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.URL.Path == "/auth/v1/signup":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"email": "user@example.com",
				},
			})
		case r.URL.Path == "/auth/v1/token" && r.URL.RawQuery == "grant_type=password":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "token-1",
				"refresh_token": "refresh-1",
				"expires_in":    3600,
				"user": map[string]any{
					"id":    "user-1",
					"email": "user@example.com",
				},
			})
		default:
			t.Fatalf("unexpected auth request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "anon-key")
	session, err := client.SignUp(context.Background(), platformapi.AuthRequest{
		Email:    "user@example.com",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	if session.AccessToken != "token-1" {
		t.Fatalf("AccessToken = %q, want %q", session.AccessToken, "token-1")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want signup + login fallback", calls)
	}
}

func TestSignUpReturnsActionableErrorWhenSupabaseCannotCreateSession(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/v1/signup":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"email": "user@example.com",
				},
			})
		case r.URL.Path == "/auth/v1/token" && r.URL.RawQuery == "grant_type=password":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Email not confirmed"}`))
		default:
			t.Fatalf("unexpected auth request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "anon-key")
	_, err := client.SignUp(context.Background(), platformapi.AuthRequest{
		Email:    "user@example.com",
		Password: "secret",
	})
	if err == nil {
		t.Fatal("expected SignUp() to fail when Supabase does not return a usable session")
	}

	apiErr, ok := err.(*platformapi.APIError)
	if !ok {
		t.Fatalf("error type = %T, want *platformapi.APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(apiErr.Message, "Confirm email") {
		t.Fatalf("Message = %q, want actionable confirm-email guidance", apiErr.Message)
	}
}
