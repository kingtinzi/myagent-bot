package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig_PlatformAPI(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PlatformAPI.BaseURL == "" {
		t.Fatal("PlatformAPI.BaseURL should be set by default")
	}
	if cfg.PlatformAPI.TimeoutSeconds <= 0 {
		t.Fatalf("PlatformAPI.TimeoutSeconds = %d, want positive", cfg.PlatformAPI.TimeoutSeconds)
	}
	if got := cfg.Agents.Defaults.Workspace; got != "workspace" {
		t.Fatalf("default workspace = %q, want %q", got, "workspace")
	}
}

func TestResolveWorkspacePathUsesPinchBotHomeForRelativePaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".pinchbot")
	t.Setenv("PINCHBOT_HOME", home)

	got := ResolveWorkspacePath("workspace")
	want := filepath.Join(home, "workspace")
	if got != want {
		t.Fatalf("ResolveWorkspacePath() = %q, want %q", got, want)
	}
}

func TestLoadOrInitConfigCreatesDefaultConfigFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), ".pinchbot", "config.json")
	t.Setenv("PINCHBOT_CONFIG", cfgPath)
	t.Setenv("PINCHBOT_HOME", filepath.Dir(cfgPath))

	cfg, err := LoadOrInitConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadOrInitConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadOrInitConfig() returned nil config")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config file to be created, err = %v", err)
	}
	if got := cfg.Agents.Defaults.Workspace; got != "workspace" {
		t.Fatalf("default workspace = %q, want %q", got, "workspace")
	}
}
