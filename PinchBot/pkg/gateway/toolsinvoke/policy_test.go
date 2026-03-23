package toolsinvoke

import (
	"net/http/httptest"
	"testing"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
)

func TestIsGatewayHTTPDenied_Defaults(t *testing.T) {
	cfg := &config.Config{}
	if !IsGatewayHTTPDenied(cfg, "gateway") {
		t.Fatal("gateway should be denied by default")
	}
	if !IsGatewayHTTPDenied(cfg, "cron") {
		t.Fatal("cron should be denied")
	}
	if IsGatewayHTTPDenied(cfg, "read_file") {
		t.Fatal("read_file should not be denied by default list")
	}
}

func TestIsGatewayHTTPDenied_AllowRemovesDefault(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Tools: &config.GatewayHTTPToolsConfig{
				Allow: []string{"cron"},
			},
		},
	}
	if IsGatewayHTTPDenied(cfg, "cron") {
		t.Fatal("cron should be allowed when listed in gateway.tools.allow")
	}
}

func TestIsGatewayHTTPDenied_ExtraDeny(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Tools: &config.GatewayHTTPToolsConfig{
				Deny: []string{"read_file"},
			},
		},
	}
	if !IsGatewayHTTPDenied(cfg, "read_file") {
		t.Fatal("read_file should be denied via gateway.tools.deny")
	}
}

func TestCheckGatewayInvokeAuth_None(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{Auth: &config.GatewayHTTPAuthConfig{Mode: "none"}}}
	if !CheckGatewayInvokeAuth(cfg, "") {
		t.Fatal("none should allow")
	}
}

func TestCheckGatewayInvokeAuth_Token(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "token", Token: "secret"},
		},
	}
	if CheckGatewayInvokeAuth(cfg, "wrong") {
		t.Fatal("wrong token")
	}
	if !CheckGatewayInvokeAuth(cfg, "secret") {
		t.Fatal("good token")
	}
}

func TestCheckGatewayInvokeAuth_Password(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Auth: &config.GatewayHTTPAuthConfig{Mode: "password", Password: "pw"},
		},
	}
	if CheckGatewayInvokeAuth(cfg, "wrong") {
		t.Fatal("wrong password")
	}
	if !CheckGatewayInvokeAuth(cfg, "pw") {
		t.Fatal("good password")
	}
}

func TestMergeToolInvokeArgs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string"},
			"x":      map[string]any{"type": "number"},
		},
	}
	args := map[string]any{"x": 1.0}
	out := MergeToolInvokeArgs(schema, "run", args)
	if out["action"] != "run" {
		t.Fatalf("got %#v", out)
	}
}

func TestBearerFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if BearerFromRequest(req) != "" {
		t.Fatal("expected empty")
	}
	req.Header.Set("Authorization", "Bearer  tok")
	if BearerFromRequest(req) != "tok" {
		t.Fatalf("got %q", BearerFromRequest(req))
	}
	if BearerFromRequest(nil) != "" {
		t.Fatal("nil request")
	}
}

func TestMergeToolInvokeArgs_NoActionProperty(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{}},
	}
	out := MergeToolInvokeArgs(schema, "run", map[string]any{"x": 1})
	if _, ok := out["action"]; ok {
		t.Fatal("should not inject action")
	}
}

func TestDeniedByAgentToolsProfile_Deny(t *testing.T) {
	p := &config.AgentToolsProfile{Deny: []string{"spawn"}}
	if !config.DeniedByAgentToolsProfile(p, "spawn") {
		t.Fatal("spawn denied")
	}
	if config.DeniedByAgentToolsProfile(p, "read_file") {
		t.Fatal("read_file not denied")
	}
}

func TestDeniedByAgentToolsProfile_Whitelist(t *testing.T) {
	p := &config.AgentToolsProfile{Allow: []string{"a"}, AlsoAllow: []string{"B"}}
	if !config.DeniedByAgentToolsProfile(p, "other") {
		t.Fatal("other should be denied when allow is set")
	}
	if config.DeniedByAgentToolsProfile(p, "a") {
		t.Fatal("a allowed")
	}
	if config.DeniedByAgentToolsProfile(p, "b") {
		t.Fatal("b allowed via alsoAllow case fold")
	}
}

func TestResolvedAgentToolsProfile_Merge(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Tools: &config.AgentToolsProfile{Deny: []string{"cron"}},
			},
			List: []config.AgentConfig{
				{ID: "beta", Tools: &config.AgentToolsProfile{Allow: []string{"read_file"}}},
			},
		},
	}
	p := agent.ResolvedAgentToolsProfile(cfg, "beta")
	if p == nil {
		t.Fatal("profile")
	}
	// Whitelist is only read_file; spawn is blocked by allow list.
	if !config.DeniedByAgentToolsProfile(p, "spawn") {
		t.Fatal("spawn should be denied by allow whitelist")
	}
	if !config.DeniedByAgentToolsProfile(p, "cron") {
		t.Fatal("merged deny should block cron")
	}
	if config.DeniedByAgentToolsProfile(p, "read_file") {
		t.Fatal("read_file in allow")
	}
}
