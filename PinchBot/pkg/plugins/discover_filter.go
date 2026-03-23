package plugins

import "strings"

// Plugins implemented in Go register with the same id as an OpenClaw extension;
// exclude them from the Node host so a broken/missing TS shim is never loaded.
// graph-memory is Go-only (pkg/graphmemory); the legacy TypeScript extension was removed.
var nativeGoPluginExclusiveNodeIDs = []string{"llm-task", "graph-memory"}

func excludePluginIDs(in []DiscoveredPlugin, drop []string) []DiscoveredPlugin {
	if len(in) == 0 || len(drop) == 0 {
		return in
	}
	dropSet := make(map[string]struct{}, len(drop))
	for _, id := range drop {
		k := strings.ToLower(strings.TrimSpace(id))
		if k != "" {
			dropSet[k] = struct{}{}
		}
	}
	out := make([]DiscoveredPlugin, 0, len(in))
	for _, p := range in {
		if _, ok := dropSet[strings.ToLower(strings.TrimSpace(p.ID))]; ok {
			continue
		}
		out = append(out, p)
	}
	return out
}
