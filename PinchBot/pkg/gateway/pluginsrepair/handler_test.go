package pluginsrepair

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestHandler_Method(t *testing.T) {
	h := &Handler{Cfg: &config.Config{}, AgentReg: func() *agent.AgentRegistry { return testRegistry(t) }}
	req := httptest.NewRequest(http.MethodGet, "/plugins/repair", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandler_Unauthorized(t *testing.T) {
	h := &Handler{
		Cfg: &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"}}},
		AgentReg: func() *agent.AgentRegistry { return testRegistry(t) },
	}
	body := []byte(`{"plugin_id":"lobster","action_id":"explain_fix"}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/repair", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_ExplainOnly(t *testing.T) {
	h := &Handler{
		Cfg: &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"}}},
		AgentReg: func() *agent.AgentRegistry { return testRegistry(t) },
	}
	body := []byte(`{"plugin_id":"lobster","action_id":"install_bundled_cli"}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/repair", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("response not ok: %v", out)
	}
}

func TestHandler_InstallBundledCLI_NeedsApproval(t *testing.T) {
	h := &Handler{
		Cfg: &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"}}},
		AgentReg: func() *agent.AgentRegistry { return testRegistry(t) },
	}
	body := []byte(`{"plugin_id":"lobster","action_id":"install_bundled_cli"}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/repair", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if st, _ := out["status"].(string); st != "needs_approval" {
		t.Fatalf("status=%v, want needs_approval", out["status"])
	}
}

func TestHandler_InstallBundledCLI_Approved_UsesInjectedFunc(t *testing.T) {
	prevInstallCli := installBundledCliFn
	installBundledCliFn = func(parentCtx context.Context, pluginID string) (string, string, error) {
		if pluginID != "lobster" {
			return "", "", errors.New("bad plugin")
		}
		return "npm install -g @clawdbot/lobster", "lobster v0.12.3", nil
	}
	defer func() { installBundledCliFn = prevInstallCli }()

	h := &Handler{
		Cfg: &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"}}},
		AgentReg: func() *agent.AgentRegistry { return testRegistry(t) },
	}
	body := []byte(`{"plugin_id":"lobster","action_id":"install_bundled_cli","args":{"approve":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/repair", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if st, _ := out["status"].(string); st != "ok" {
		t.Fatalf("status=%v, want ok", out["status"])
	}
}

func TestHandler_InstallNodeDeps_UsesInjectedFuncs(t *testing.T) {
	prevResolve := resolvePluginRootFn
	prevInstall := installNodeDepsFn
	resolvePluginRootFn = func(workspace string, cfg *config.Config, pluginID string) (string, error) {
		if pluginID != "lobster" {
			return "", errors.New("bad plugin")
		}
		return "C:/tmp/lobster", nil
	}
	installNodeDepsFn = func(parentCtx context.Context, pluginRoot string) (string, string, error) {
		if pluginRoot != "C:/tmp/lobster" {
			return "", "", errors.New("bad root")
		}
		return "npm ci", "ok", nil
	}
	defer func() {
		resolvePluginRootFn = prevResolve
		installNodeDepsFn = prevInstall
	}()

	h := &Handler{
		Cfg: &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"}}},
		AgentReg: func() *agent.AgentRegistry { return testRegistry(t) },
	}
	body := []byte(`{"plugin_id":"lobster","action_id":"install_node_deps"}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/repair", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("response not ok: %v", out)
	}
}

