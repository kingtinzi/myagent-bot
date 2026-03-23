package toolsinvoke

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		Cfg:      &config.Config{},
		AgentReg: func() *agent.AgentRegistry { return nil },
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tools/invoke", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestHandler_RateLimited(t *testing.T) {
	tok := "rate-limit-handler-" + t.Name()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: tok},
			RateLimit: &config.GatewayRateLimitConfig{
				RequestsPerMinute: 1,
			},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return nil }}
	body := `{"tool":"read_file","args":{}}`
	req1 := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+tok)
	req1.RemoteAddr = "198.51.100.77:1"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code == http.StatusTooManyRequests {
		t.Fatalf("unexpected 429 on first request: %s", rec1.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+tok)
	req2.RemoteAddr = "198.51.100.77:1"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on second request, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandler_UnauthorizedToken(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return nil }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"read_file","args":{}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_DeniedToolNoRegistry(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return nil }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"gateway"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_MissingToolName(t *testing.T) {
	reg := agent.NewAgentRegistry(&config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				Model:             "m",
				MaxTokens:         100,
				MaxToolIterations: 3,
			},
		},
	}, invokeTestMockProvider{})
	h := &Handler{Cfg: &config.Config{}, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"args":{}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
}

// invokeTestMockProvider is a minimal LLMProvider for handler_test (no import cycle with integration file).
type invokeTestMockProvider struct{}

func (invokeTestMockProvider) Chat(ctx context.Context, messages []providers.Message, toolsDef []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "x", FinishReason: "stop"}, nil
}
func (invokeTestMockProvider) GetDefaultModel() string { return "m" }

func TestReadLimitedJSONBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	b, err := readLimitedJSONBody(r, 1024)
	if err != nil || string(b) != `{"a":1}` {
		t.Fatalf("err=%v b=%s", err, b)
	}
}
