package plugins

import (
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/tools"
)

// ManagedPluginHost is kept as a compatibility placeholder after TS extension removal.
type ManagedPluginHost struct{}

// RegisterNodeHostTools is deprecated and now a no-op.
func RegisterNodeHostTools(
	reg *tools.ToolRegistry,
	cfg *config.Config,
	workspace string,
	sandboxed bool,
) (stop func(), lobsterFromNode bool, host *ManagedPluginHost, err error) {
	_ = reg
	_ = cfg
	_ = workspace
	_ = sandboxed
	return func() {}, false, nil, nil
}

// effectiveNodeHostEnabled is deprecated and always empty after TS runtime removal.
func effectiveNodeHostEnabled(cfg *config.Config) []string {
	_ = cfg
	return nil
}

// LogGraphMemoryStartupStatus logs whether the Go-native graph-memory runtime will run.
func LogGraphMemoryStartupStatus(cfg *config.Config, host *ManagedPluginHost) {
	_ = host
	if cfg == nil {
		return
	}
	if GraphMemoryRuntimeActive(cfg) {
		logger.InfoCF("plugins", "graph-memory: active", map[string]any{"mode": "go-native"})
		return
	}
	var parts []string
	if cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		parts = append(parts, `config.graph-memory.json missing next to config.json, or "enabled": false`)
	}
	if !cfg.Plugins.IsPluginEnabled(DefaultGraphMemoryEngineID) {
		parts = append(parts, "graph-memory not listed in plugins.enabled")
	}
	logger.InfoCF("plugins", "graph-memory: inactive", map[string]any{"detail": strings.Join(parts, "; ")})
}
