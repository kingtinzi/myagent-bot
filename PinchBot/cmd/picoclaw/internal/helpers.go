package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sipeed/pinchbot/pkg/config"
)

const Logo = "🦞"

var (
	version   = "dev"
	gitCommit string
	buildTime string
	goVersion string
)

// GetPinchBotHome returns the PinchBot data directory (config + workspace).
// Priority: ${PINCHBOT_HOME} > <exe_dir>/.pinchbot（与程序同目录，便于管理）
func GetPinchBotHome() string {
	if home := os.Getenv("PINCHBOT_HOME"); home != "" {
		return home
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".pinchbot")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pinchbot")
}

// GetConfigPath returns the path to config.json (inside GetPinchBotHome()).
// Priority: PINCHBOT_CONFIG env > <pinchbot_home>/config.json
func GetConfigPath() string {
	if configPath := os.Getenv("PINCHBOT_CONFIG"); configPath != "" {
		return configPath
	}
	return filepath.Join(GetPinchBotHome(), "config.json")
}

// ResolveWorkspacePath returns the absolute workspace path. If config's workspace is relative,
// it is resolved against GetPinchBotHome() (e.g. "workspace" -> .pinchbot/workspace).
func ResolveWorkspacePath(cfg *config.Config) string {
	p := cfg.WorkspacePath()
	if p == "" {
		return filepath.Join(GetPinchBotHome(), "workspace")
	}
	if !filepath.IsAbs(p) {
		return filepath.Join(GetPinchBotHome(), p)
	}
	return p
}

// EnsurePinchBotHome creates .pinchbot and default config.json in exe directory if not present.
// 程序启动时在程序所在目录下没有则生成，有则不生成。
func EnsurePinchBotHome() error {
	path := GetConfigPath()
	return config.EnsureConfigAt(path)
}

func LoadConfig() (*config.Config, error) {
	return config.LoadConfig(GetConfigPath())
}

// FormatVersion returns the version string with optional git commit
func FormatVersion() string {
	v := version
	if gitCommit != "" {
		v += fmt.Sprintf(" (git: %s)", gitCommit)
	}
	return v
}

// FormatBuildInfo returns build time and go version info
func FormatBuildInfo() (string, string) {
	build := buildTime
	goVer := goVersion
	if goVer == "" {
		goVer = runtime.Version()
	}
	return build, goVer
}

// GetVersion returns the version string
func GetVersion() string {
	return version
}
