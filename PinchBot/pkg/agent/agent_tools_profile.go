package agent

import (
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
	"github.com/sipeed/pinchbot/pkg/routing"
	"github.com/sipeed/pinchbot/pkg/tools"
)

// ResolvedAgentToolsProfile merges agents.defaults.tools with agents.list[].tools for agentID (normalized).
func ResolvedAgentToolsProfile(cfg *config.Config, agentID string) *config.AgentToolsProfile {
	if cfg == nil {
		return nil
	}
	id := routing.NormalizeAgentID(agentID)
	var entry *config.AgentToolsProfile
	for i := range cfg.Agents.List {
		if routing.NormalizeAgentID(cfg.Agents.List[i].ID) == id {
			entry = cfg.Agents.List[i].Tools
			break
		}
	}
	return config.MergeAgentToolsProfile(cfg.Agents.Defaults.Tools, entry)
}

// FilterProviderToolDefsByAgentProfile removes tools blocked by the merged per-agent profile (same rules as POST /tools/invoke).
func FilterProviderToolDefsByAgentProfile(cfg *config.Config, agentID string, defs []providers.ToolDefinition) []providers.ToolDefinition {
	if cfg == nil || len(defs) == 0 {
		return defs
	}
	p := ResolvedAgentToolsProfile(cfg, agentID)
	return tools.FilterProviderToolDefsByMergedProfile(p, defs)
}
