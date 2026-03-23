package toolsinvoke

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
	"github.com/sipeed/pinchbot/pkg/routing"
	"github.com/sipeed/pinchbot/pkg/tools"
)

type invokeMockProvider struct{}

func (invokeMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	toolsDef []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok", FinishReason: "stop"}, nil
}

func (invokeMockProvider) GetDefaultModel() string { return "mock" }

// invokeEchoTool is registered only in tests (unique name).
type invokeEchoTool struct{}

func (invokeEchoTool) Name() string        { return "invoke_echo_test_tool" }
func (invokeEchoTool) Description() string { return "test echo for tools/invoke" }
func (invokeEchoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"msg":    map[string]any{"type": "string"},
			"action": map[string]any{"type": "string"},
		},
	}
}

func (invokeEchoTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	msg, _ := args["msg"].(string)
	act, _ := args["action"].(string)
	if act != "" {
		msg = act + ":" + msg
	}
	return tools.SilentResult(msg)
}

type invokeFailTool struct{}

func (invokeFailTool) Name() string        { return "invoke_fail_test_tool" }
func (invokeFailTool) Description() string { return "always errors" }
func (invokeFailTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (invokeFailTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.ErrorResult("intentional failure")
}

func newInvokeTestRegistry(t *testing.T, agentList []config.AgentConfig) (*agent.AgentRegistry, *config.Config) {
	t.Helper()
	ws := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:           ws,
				Model:               "mock",
				MaxTokens:           1024,
				MaxToolIterations:   5,
				RestrictToWorkspace: false,
			},
			List: agentList,
		},
		Plugins: config.PluginsConfig{
			NodeHost: false,
			Enabled:  nil,
		},
	}
	return agent.NewAgentRegistry(cfg, invokeMockProvider{}), cfg
}

func TestHandler_InvokeSuccess(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{
		Cfg:      cfg,
		AgentReg: func() *agent.AgentRegistry { return reg },
	}
	rr := httptest.NewRecorder()
	body := `{"tool":"invoke_echo_test_tool","args":{"msg":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("ok: %#v", out)
	}
	res, _ := out["result"].(map[string]any)
	if res == nil {
		t.Fatalf("missing result: %v", out)
	}
	if res["content"] != "hello" {
		t.Fatalf("content = %#v", res["content"])
	}
}

func TestHandler_DryRun(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	body := `{"tool":"invoke_echo_test_tool","args":{"msg":"hello"},"dryRun":true}`
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true || out["dryRun"] != true {
		t.Fatalf("out: %#v", out)
	}
	res, _ := out["result"].(map[string]any)
	if res["content"] == "hello" {
		t.Fatal("dry run must not execute tool")
	}
	rv, _ := out["resolved"].(map[string]any)
	if rv == nil {
		t.Fatal("missing resolved")
	}
	ma, _ := rv["merged_args"].(map[string]any)
	if ma["msg"] != "hello" {
		t.Fatalf("merged_args: %#v", ma)
	}
}

func TestHandler_InvokeActionMerged(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	body := `{"tool":"invoke_echo_test_tool","action":"run","args":{"msg":"x"}}`
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	res, _ := out["result"].(map[string]any)
	if res["content"] != "run:x" {
		t.Fatalf("got %#v", res["content"])
	}
}

func TestHandler_InvokeSessionKeyRoutesAgent(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{
		{ID: "main", Default: true},
		{ID: "beta", Default: false},
	})
	beta, _ := reg.GetAgent("beta")
	beta.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	sk := routing.BuildAgentMainSessionKey("beta")
	rr := httptest.NewRecorder()
	body := `{"tool":"invoke_echo_test_tool","args":{"msg":"from-beta"},"sessionKey":` + jsonString(sk) + `}`
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	res, _ := out["result"].(map[string]any)
	if res["content"] != "from-beta" {
		t.Fatalf("got %#v", res["content"])
	}

	// main agent does not have the tool
	rr2 := httptest.NewRecorder()
	body2 := `{"tool":"invoke_echo_test_tool","args":{"msg":"nope"}}`
	req2 := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for main without tool, got %d", rr2.Code)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestHandler_InvokeToolErrorStatus(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeFailTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_fail_test_tool"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_BodyTooLarge(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	padding := strings.Repeat("x", maxBodyBytes+50)
	body := `{"tool":"invoke_echo_test_tool","args":{"msg":"` + padding + `"}}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_PasswordAuth(t *testing.T) {
	reg, baseCfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	baseCfg.Gateway = config.GatewayConfig{
		Auth: &config.GatewayHTTPAuthConfig{Mode: "password", Password: "p1"},
	}
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: baseCfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_echo_test_tool","args":{"msg":"ok"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer p1")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_MessageChannelHeader(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{{ID: "main", Default: true}})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(toolChannelProbeTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_channel_probe"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-openclaw-message-channel", "telegram")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	res, _ := out["result"].(map[string]any)
	if res["content"] != "channel=telegram" {
		t.Fatalf("got %#v", res["content"])
	}
}

type toolChannelProbeTool struct{}

func (toolChannelProbeTool) Name() string        { return "invoke_channel_probe" }
func (toolChannelProbeTool) Description() string { return "probe" }
func (toolChannelProbeTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (toolChannelProbeTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.SilentResult("channel=" + tools.ToolChannel(ctx))
}

func TestHandler_AgentToolsDeny(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{
		{ID: "main", Default: true, Tools: &config.AgentToolsProfile{Deny: []string{"invoke_echo_test_tool"}}},
	})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_echo_test_tool","args":{"msg":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_AgentToolsAllowWhitelist(t *testing.T) {
	reg, cfg := newInvokeTestRegistry(t, []config.AgentConfig{
		{ID: "main", Default: true, Tools: &config.AgentToolsProfile{Allow: []string{"invoke_echo_test_tool"}}},
	})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})
	a.Tools.Register(invokeFailTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_fail_test_tool"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for tool not in allow list, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_AgentDefaultsToolsMergedDeny(t *testing.T) {
	ws := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:           ws,
				Model:               "mock",
				MaxTokens:           1024,
				MaxToolIterations:   5,
				RestrictToWorkspace: false,
				Tools:               &config.AgentToolsProfile{Deny: []string{"invoke_echo_test_tool"}},
			},
			List: []config.AgentConfig{{ID: "main", Default: true}},
		},
		Plugins: config.PluginsConfig{NodeHost: false},
	}
	reg := agent.NewAgentRegistry(cfg, invokeMockProvider{})
	a, _ := reg.GetAgent("main")
	a.Tools.Register(invokeEchoTool{})

	h := &Handler{Cfg: cfg, AgentReg: func() *agent.AgentRegistry { return reg }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", strings.NewReader(`{"tool":"invoke_echo_test_tool"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
