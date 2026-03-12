package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	PinchBotHomeEnv   = "PINCHBOT_HOME"
	PinchBotConfigEnv = "PINCHBOT_CONFIG"
	LegacyHomeEnv     = "PICOCLAW_HOME"
	LegacyConfigEnv   = "PICOCLAW_CONFIG"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func executableDir() string {
	if exePath, err := os.Executable(); err == nil && strings.TrimSpace(exePath) != "" {
		return filepath.Dir(exePath)
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return "."
}

func GetPinchBotHome() string {
	if home := firstNonEmpty(os.Getenv(PinchBotHomeEnv), os.Getenv(LegacyHomeEnv)); home != "" {
		return filepath.Clean(home)
	}
	return filepath.Join(executableDir(), ".pinchbot")
}

func GetConfigPath() string {
	if configPath := firstNonEmpty(os.Getenv(PinchBotConfigEnv), os.Getenv(LegacyConfigEnv)); configPath != "" {
		return filepath.Clean(configPath)
	}
	return filepath.Join(GetPinchBotHome(), "config.json")
}

func ResolveWorkspacePath(path string) string {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(GetPinchBotHome(), path)
}

func LoadOrInitConfig(path string) (*Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err := SaveConfig(path, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
