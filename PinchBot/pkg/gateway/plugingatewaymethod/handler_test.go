package plugingatewaymethod

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

type testProvider struct{}

func (testProvider) Chat(ctx context.Context, messages []providers.Message, toolsDef []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "x", FinishReason: "stop"}, nil
}
func (testProvider) GetDefaultModel() string { return "m" }

func testRegistry(t *testing.T) *agent.AgentRegistry {
	t.Helper()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				Model:             "m",
				MaxTokens:         100,
				MaxToolIterations: 3,
			},
		},
	}
	return agent.NewAgentRegistry(cfg, testProvider{})
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := &Handler{Cfg: &config.Config{}, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/gateway-method", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandler_UnauthorizedWhenTokenRequired(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodPost, "/plugins/gateway-method", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandler_NoPluginHost(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodPost, "/plugins/gateway-method", strings.NewReader(`{"plugin_id":"p","method":"m","params":{}}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["ok"] != false {
		t.Fatalf("expected ok false: %v", out)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodPost, "/plugins/gateway-method", strings.NewReader(`{`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_MissingPluginIDOrMethod(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodPost, "/plugins/gateway-method", strings.NewReader(`{"params":{}}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

// Second request in the same minute returns 429 when rpm=1 (shared global limiter with other Gateway surfaces).
func TestHandler_RateLimitSecondRequest429(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth:      &config.GatewayHTTPAuthConfig{Mode: "token", Token: "gw-plugingw-ratelimit-test-token"},
			RateLimit: &config.GatewayRateLimitConfig{RequestsPerMinute: 1},
		},
	}
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	body := `{"plugin_id":"p","method":"m","params":{}}`
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/plugins/gateway-method", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer gw-plugingw-ratelimit-test-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	r1 := do()
	if r1.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate limited, got %d %s", r1.Code, r1.Body.String())
	}
	r2 := do()
	if r2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d body %s", r2.Code, r2.Body.String())
	}
}
