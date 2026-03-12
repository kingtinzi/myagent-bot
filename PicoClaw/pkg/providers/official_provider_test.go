package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/platformapi"
)

func TestCreateProviderFromConfig_Official(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "official-basic",
		Model:     "official/official-basic",
		APIBase:   "https://platform.example.com",
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if modelID != "official-basic" {
		t.Fatalf("modelID = %q, want %q", modelID, "official-basic")
	}
	if _, ok := provider.(*OfficialProvider); !ok {
		t.Fatalf("provider type = %T, want *OfficialProvider", provider)
	}
}

func TestOfficialProviderChatRequiresUserToken(t *testing.T) {
	provider := NewOfficialProvider("https://platform.example.com")

	_, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, "official-basic", map[string]any{})
	if err == nil {
		t.Fatal("Chat() expected auth token error")
	}
}

func TestOfficialProviderChatCallsPlatformEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer session-token")
		}
		if r.URL.Path != "/chat/official" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/chat/official")
		}
		var req platformapi.ChatProxyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ModelID != "official-basic" {
			t.Fatalf("model_id = %q, want %q", req.ModelID, "official-basic")
		}
		if _, ok := req.Options["user_access_token"]; ok {
			t.Fatal("request options unexpectedly include user_access_token")
		}
		_ = json.NewEncoder(w).Encode(platformapi.ChatProxyResponse{
			Response: LLMResponse{
				Content: "official reply",
				Usage: &UsageInfo{
					PromptTokens:     100,
					CompletionTokens: 20,
					TotalTokens:      120,
				},
			},
			ChargedFen: 12,
		})
	}))
	defer server.Close()

	provider := NewOfficialProvider(server.URL)
	resp, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, "official-basic", map[string]any{
		"user_access_token": "session-token",
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Content != "official reply" {
		t.Fatalf("content = %q, want %q", resp.Content, "official reply")
	}
}

func TestOfficialProviderChatIncludesUpstreamErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "insufficient wallet balance", http.StatusPaymentRequired)
	}))
	defer server.Close()

	provider := NewOfficialProvider(server.URL)
	_, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, "official-basic", map[string]any{
		"user_access_token": "session-token",
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "insufficient wallet balance") {
		t.Fatalf("error = %q, want status code and upstream message", err.Error())
	}
	var apiErr *platformapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *platformapi.APIError", err)
	}
	if apiErr.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusPaymentRequired)
	}
}
