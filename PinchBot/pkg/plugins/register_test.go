package plugins

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestEffectiveNodeHostEnabled_graphMemoryNeverInNodeHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Plugins.Enabled = []string{"graph-memory", "llm-task"}
	cfg.GraphMemory = nil
	got := effectiveNodeHostEnabled(cfg)
	if len(got) != 0 {
		t.Fatalf("expected no node-host plugins, got %#v", got)
	}

	cfg.GraphMemory = &config.GraphMemoryFileConfig{Enabled: true}
	got2 := effectiveNodeHostEnabled(cfg)
	if len(got2) != 0 {
		t.Fatalf("expected no node-host plugins, got %#v", got2)
	}
}
