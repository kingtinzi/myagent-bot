package plugins

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

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

