package plugins

import (
	"context"
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestSessionEndGraphMemoryNoPanicWhenDisabled(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Enabled:             []string{"graph-memory"},
			GraphMemoryGoNative: true,
		},
		GraphMemory: &config.GraphMemoryFileConfig{
			Enabled: true,
			Raw: map[string]any{
				"dbPath": ":memory:",
			},
		},
	}
	SessionEndGraphMemory(context.Background(), cfg, "s-test")
}

