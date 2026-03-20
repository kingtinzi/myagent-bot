package plugins

import (
	"context"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/tools"
)

// RegisterNodeHostTools starts the Node host, loads enabled extensions, and registers bridge tools.
// The returned stop function should be called when the owning agent shuts down (e.g. gateway Stop).
// host is non-nil when the managed Node process is running (even if the catalog is empty — caller may still use ContextOp when extensions registered engines only).
// Returns true if a tool named "lobster" was registered from the Node catalog (so Go lobster can be skipped).
func RegisterNodeHostTools(
	reg *tools.ToolRegistry,
	cfg *config.Config,
	workspace string,
	sandboxed bool,
) (stop func(), lobsterFromNode bool, host *ManagedPluginHost, err error) {
	stop = func() {}
	if cfg == nil || reg == nil {
		return stop, false, nil, nil
	}
	if !cfg.Plugins.NodeHost {
		return stop, false, nil, nil
	}
	enabledForHost := effectiveNodeHostEnabled(cfg)
	if len(enabledForHost) == 0 {
		return stop, false, nil, nil
	}

	extRoot, err := ResolveExtensionsDir(workspace, cfg.Plugins.ExtensionsDir)
	if err != nil {
		return stop, false, nil, err
	}
	discovered, err := DiscoverEnabled(extRoot, enabledForHost)
	if err != nil {
		return stop, false, nil, err
	}
	discovered = excludePluginIDs(discovered, nativeGoPluginExclusiveNodeIDs)
	if len(discovered) == 0 {
		return stop, false, nil, nil
	}

	for i := range discovered {
		if cfg.GraphMemory != nil && cfg.GraphMemory.Enabled &&
			strings.EqualFold(strings.TrimSpace(discovered[i].ID), DefaultGraphMemoryEngineID) &&
			len(cfg.GraphMemory.Raw) > 0 {
			discovered[i].PluginConfig = cfg.GraphMemory.Raw
		}
	}

	nodeBin := strings.TrimSpace(cfg.Plugins.NodeBinary)
	hostDir := strings.TrimSpace(cfg.Plugins.HostDir)

	startRetries := cfg.Plugins.NodeHostStartRetries
	if startRetries <= 0 {
		startRetries = 3
	}

	restartDelay := time.Duration(cfg.Plugins.NodeHostRestartDelayMs) * time.Millisecond
	if cfg.Plugins.NodeHostRestartDelayMs <= 0 {
		restartDelay = 500 * time.Millisecond
	}

	opts := ManagedHostOpts{
		NodeBinary:     nodeBin,
		HostDir:        hostDir,
		Workspace:      workspace,
		Sandboxed:      sandboxed,
		Discovered:     discovered,
		PinchbotConfig: BuildPinchbotPluginAPIConfig(cfg),
		MaxRecoveries:  cfg.Plugins.NodeHostMaxRecoveries,
		RestartDelay:   restartDelay,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var managed *ManagedPluginHost
	var catalog []CatalogTool
	var lastErr error
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt < startRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return stop, false, nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(minInt(2000, int(backoff.Milliseconds()*2))) * time.Millisecond
		}
		managed, catalog, lastErr = BootstrapManagedHost(ctx, opts)
		if lastErr == nil {
			break
		}
		if managed != nil {
			_ = managed.Close()
			managed = nil
		}
	}
	if managed == nil {
		if lastErr != nil {
			return stop, false, nil, lastErr
		}
		return stop, false, nil, nil
	}

	stop = func() { _ = managed.Close() }

	for _, ct := range catalog {
		if ct.Name == "" {
			continue
		}
		reg.Register(NewBridgeTool(managed, ct.PluginID, ct.Name, ct.Description, ct.Parameters))
		if strings.EqualFold(ct.Name, "lobster") {
			lobsterFromNode = true
		}
	}
	return stop, lobsterFromNode, managed, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// effectiveNodeHostEnabled returns plugin IDs to load in the Node host. graph-memory is only
// loaded when config.graph-memory.json exists with enabled=true (cfg.GraphMemory), so a default
// config can list graph-memory without requiring the sidecar until the user opts in.
func effectiveNodeHostEnabled(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Plugins.Enabled))
	gmActive := cfg.GraphMemory != nil && cfg.GraphMemory.Enabled
	for _, id := range cfg.Plugins.Enabled {
		raw := strings.TrimSpace(id)
		if raw == "" {
			continue
		}
		if strings.EqualFold(raw, DefaultGraphMemoryEngineID) && !gmActive {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// LogGraphMemoryStartupStatus logs whether recall/assembly will run (sidecar enabled + node host running).
func LogGraphMemoryStartupStatus(cfg *config.Config, host *ManagedPluginHost) {
	if cfg == nil || !cfg.Plugins.NodeHost {
		return
	}
	if GraphMemoryRuntimeActive(cfg, host) {
		logger.InfoCF("plugins", "graph-memory: active", nil)
		return
	}
	var parts []string
	if cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		parts = append(parts, `config.graph-memory.json missing next to config.json, or "enabled": false`)
	}
	if len(effectiveNodeHostEnabled(cfg)) == 0 {
		parts = append(parts, "no Node extensions to load (graph-memory is skipped until sidecar enables it)")
	} else if host == nil {
		parts = append(parts, "node plugin host did not start")
	}
	if !cfg.Plugins.IsPluginEnabled(DefaultGraphMemoryEngineID) {
		parts = append(parts, "graph-memory not listed in plugins.enabled")
	}
	logger.InfoCF("plugins", "graph-memory: inactive", map[string]any{"detail": strings.Join(parts, "; ")})
}
