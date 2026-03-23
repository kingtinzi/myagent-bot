package plugins

import "github.com/sipeed/pinchbot/pkg/config"

// BuildPinchbotPluginAPIConfig builds a minimal api.config object for OpenClaw-style plugins
// (e.g. graph-memory readProviderModel reads agents.defaults.model.primary).
func BuildPinchbotPluginAPIConfig(cfg *config.Config) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	primary := cfg.Agents.Defaults.GetModelName()
	return map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": map[string]any{
					"primary": primary,
				},
			},
		},
		// Exposed as api.config.runtime; Node host merges into api.runtime (see assets/run.mjs).
		"runtime": map[string]any{
			"version": "pinchbot",
			"kind":    "pinchbot",
		},
	}
}
