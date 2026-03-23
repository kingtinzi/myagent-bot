package pluginsstatus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestHandler_GET(t *testing.T) {
	cfg := &config.Config{}
	reg := testRegistry(t)
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	reg := testRegistry(t)
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	req := httptest.NewRequest(http.MethodPost, "/plugins/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandler_UnauthorizedWhenGatewayTokenRequired(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	reg := testRegistry(t)
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_GatewayRateLimitInJSON(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
			RateLimit: &config.GatewayRateLimitConfig{
				RequestsPerMinute: 42,
			},
		},
	}
	reg := testRegistry(t)
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if v, ok := out["gateway_rate_limit_requests_per_minute"].(float64); !ok || int(v) != 42 {
		t.Fatalf("expected gateway_rate_limit_requests_per_minute 42, got %#v", out["gateway_rate_limit_requests_per_minute"])
	}
}

func TestHandler_OKWithBearerWhenGatewayTokenRequired(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	reg := testRegistry(t)
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
