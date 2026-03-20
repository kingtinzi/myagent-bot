package plugins

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestGraphMemoryRecentWindowClamp(t *testing.T) {
	t.Parallel()

	if got := GraphMemoryRecentWindow(nil); got != 20 {
		t.Fatalf("nil cfg recentWindow = %d, want 20", got)
	}

	cfgLow := &config.Config{GraphMemory: &config.GraphMemoryFileConfig{Enabled: true, Raw: map[string]any{"recentWindow": float64(1)}}}
	if got := GraphMemoryRecentWindow(cfgLow); got != 4 {
		t.Fatalf("low recentWindow clamp = %d, want 4", got)
	}

	cfgHigh := &config.Config{GraphMemory: &config.GraphMemoryFileConfig{Enabled: true, Raw: map[string]any{"recentWindow": float64(1000)}}}
	if got := GraphMemoryRecentWindow(cfgHigh); got != 100 {
		t.Fatalf("high recentWindow clamp = %d, want 100", got)
	}

	cfgMid := &config.Config{GraphMemory: &config.GraphMemoryFileConfig{Enabled: true, Raw: map[string]any{"recentWindow": float64(24)}}}
	if got := GraphMemoryRecentWindow(cfgMid); got != 24 {
		t.Fatalf("mid recentWindow = %d, want 24", got)
	}
}

func TestGraphMemorySidecarDefaults(t *testing.T) {
	cfg := &config.Config{
		Plugins:     config.PluginsConfig{},
		GraphMemory: &config.GraphMemoryFileConfig{Enabled: true, Raw: map[string]any{}},
	}
	if !graphMemoryAutoExtractEnabled(cfg) {
		t.Fatal("expected nativeAutoExtract default true")
	}
	if !graphMemoryAutoMaintainEnabled(cfg) {
		t.Fatal("expected nativeAutoMaintain default true")
	}
	if got := graphMemoryMaintainEveryTurns(cfg); got != 20 {
		t.Fatalf("expected default maintainEveryTurns=20, got %d", got)
	}
}

func TestGraphMemorySidecarOverrides(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{},
		GraphMemory: &config.GraphMemoryFileConfig{
			Enabled: true,
			Raw: map[string]any{
				"nativeAutoExtract":      false,
				"nativeAutoMaintain":     false,
				"nativeMaintainEveryTurns": float64(7),
			},
		},
	}
	if graphMemoryAutoExtractEnabled(cfg) {
		t.Fatal("expected nativeAutoExtract false")
	}
	if graphMemoryAutoMaintainEnabled(cfg) {
		t.Fatal("expected nativeAutoMaintain false")
	}
	if got := graphMemoryMaintainEveryTurns(cfg); got != 7 {
		t.Fatalf("expected maintainEveryTurns=7, got %d", got)
	}
}

