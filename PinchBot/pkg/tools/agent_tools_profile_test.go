package tools

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

func TestFilterProviderToolDefsByMergedProfile(t *testing.T) {
	p := &config.AgentToolsProfile{Allow: []string{"keep"}}
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "keep"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "drop"}},
	}
	out := FilterProviderToolDefsByMergedProfile(p, defs)
	if len(out) != 1 || out[0].Function.Name != "keep" {
		t.Fatalf("got %#v", out)
	}
}
