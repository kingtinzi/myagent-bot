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
	home := filepath.Join(t.TempDir(), ".openclaw")
	t.Setenv("PINCHBOT_HOME", home)

	got := ResolveWorkspacePath("workspace")
	want := filepath.Join(home, "workspace")
	if got != want {
		t.Fatalf("ResolveWorkspacePath() = %q, want %q", got, want)
	}
}

func TestGetPinchBotHomeExpandsTildeFromEnv(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	t.Setenv("PINCHBOT_HOME", "~/pinchbot-home")
	t.Setenv("PICOCLAW_HOME", "")

	if got, want := GetPinchBotHome(), filepath.Join(homeDir, "pinchbot-home"); got != want {
		t.Fatalf("GetPinchBotHome() = %q, want %q", got, want)
	}
}

func TestGetPinchBotHomeExpandsLegacyTildeEnv(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	t.Setenv("PINCHBOT_HOME", "")
	t.Setenv("PICOCLAW_HOME", "~/legacy-pinchbot-home")

	if got, want := GetPinchBotHome(), filepath.Join(homeDir, "legacy-pinchbot-home"); got != want {
		t.Fatalf("GetPinchBotHome() = %q, want %q", got, want)
	}
}

func TestGetConfigPathExpandsTildeFromEnv(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	t.Setenv("PINCHBOT_CONFIG", "~/pinchbot/config.json")

	if got, want := GetConfigPath(), filepath.Join(homeDir, "pinchbot", "config.json"); got != want {
		t.Fatalf("GetConfigPath() = %q, want %q", got, want)
	}
}

func TestGetPinchBotHomeAnchorsRelativeEnvToExecutableDir(t *testing.T) {
	t.Setenv("PINCHBOT_HOME", "relative-home")
	t.Setenv("PICOCLAW_HOME", "")

	got := GetPinchBotHome()
	if !filepath.IsAbs(got) {
		t.Fatalf("GetPinchBotHome() = %q, want absolute path", got)
	}
	if filepath.Base(got) != "relative-home" {
		t.Fatalf("GetPinchBotHome() = %q, want basename %q", got, "relative-home")
	}
}

func TestGetConfigPathAnchorsRelativeConfigToPinchBotHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".openclaw")
	t.Setenv("PINCHBOT_HOME", home)
	t.Setenv("PINCHBOT_CONFIG", filepath.Join("custom", "config.json"))

	got := GetConfigPath()
	want := filepath.Join(home, "custom", "config.json")
	if got != want {
		t.Fatalf("GetConfigPath() = %q, want %q", got, want)
	}
}

func TestIsMacOSAppBundleExecutablePath(t *testing.T) {
	bundleExe := filepath.Join("/Applications", "launcher-chat.app", "Contents", "MacOS", "launcher-chat")
	if !isMacOSAppBundleExecutable(bundleExe) {
		t.Fatalf("expected bundle path to be detected")
	}
	if isMacOSAppBundleExecutable("/usr/bin/bash") {
		t.Fatalf("expected non-bundle path")
	}
	if isMacOSAppBundleExecutable(filepath.Join("/tmp", "foo")) {
		t.Fatalf("expected non-bundle path")
	}
}

func TestPinchBotHomeBaseForUsesExecutableDirectory(t *testing.T) {
	exe := filepath.Join("/opt", "picoclaw-darwin-amd64")
	got := pinchBotHomeBaseFor(exe)
	want := filepath.Join("/opt")
	if got != want {
		t.Fatalf("pinchBotHomeBaseFor() = %q, want %q", got, want)
	}
}

func TestLoadOrInitConfigCreatesDefaultConfigFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), ".openclaw", "config.json")
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
