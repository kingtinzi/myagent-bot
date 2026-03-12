package authbridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/platformapi"
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
