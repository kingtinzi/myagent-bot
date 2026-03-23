package agent

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

func TestFilterProviderToolDefsByAgentProfile(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			List: []config.AgentConfig{
				{ID: "main", Tools: &config.AgentToolsProfile{Allow: []string{"keep_me"}}},
			},
		},
	}
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "keep_me", Parameters: map[string]any{}}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "drop_me", Parameters: map[string]any{}}},
	}
	out := FilterProviderToolDefsByAgentProfile(cfg, "main", defs)
	if len(out) != 1 || out[0].Function.Name != "keep_me" {
		t.Fatalf("got %#v", out)
	}
}

func TestFilterProviderToolDefsByAgentProfile_NilCfg(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "x"}},
	}
	out := FilterProviderToolDefsByAgentProfile(nil, "main", defs)
	if len(out) != 1 {
		t.Fatal(out)
	}
}
