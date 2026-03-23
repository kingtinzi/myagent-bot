package plugins

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestBuildPinchbotPluginAPIConfig_Runtime(t *testing.T) {
	cfg := config.DefaultConfig()
	m := BuildPinchbotPluginAPIConfig(cfg)
	rt, ok := m["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime map, got %+v", m["runtime"])
	}
	if rt["version"] != "pinchbot" {
		t.Fatalf("version: %+v", rt)
	}
	if rt["kind"] != "pinchbot" {
		t.Fatalf("kind: %+v", rt)
	}
	agents, ok := m["agents"].(map[string]any)
	if !ok {
		t.Fatal("expected agents")
	}
	if _, ok := agents["defaults"]; !ok {
		t.Fatalf("agents: %+v", agents)
	}
}
