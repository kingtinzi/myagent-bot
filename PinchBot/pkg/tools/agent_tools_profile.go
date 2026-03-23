package tools

import (
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

// FilterProviderToolDefsByMergedProfile removes tools blocked by a merged agents.*.tools profile (same rules as main agent loop).
func FilterProviderToolDefsByMergedProfile(p *config.AgentToolsProfile, defs []providers.ToolDefinition) []providers.ToolDefinition {
	if p == nil || len(defs) == 0 {
		return defs
	}
	out := make([]providers.ToolDefinition, 0, len(defs))
	for _, d := range defs {
		name := d.Function.Name
		if config.DeniedByAgentToolsProfile(p, name) {
			continue
		}
		out = append(out, d)
	}
	return out
}
