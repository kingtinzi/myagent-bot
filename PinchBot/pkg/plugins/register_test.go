package plugins

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestEffectiveNodeHostEnabled_graphMemoryGatedBySidecar(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Plugins.Enabled = []string{"graph-memory", "lobster"}
	cfg.GraphMemory = nil
	got := effectiveNodeHostEnabled(cfg)
	if len(got) != 1 || got[0] != "lobster" {
		t.Fatalf("expected [lobster], got %#v", got)
	}

	cfg.GraphMemory = &config.GraphMemoryFileConfig{Enabled: true}
	got2 := effectiveNodeHostEnabled(cfg)
	if len(got2) != 2 {
		t.Fatalf("expected graph-memory + lobster, got %#v", got2)
	}
}
