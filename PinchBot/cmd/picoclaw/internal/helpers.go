package internal

import (
	"fmt"
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

// GetPicoclawHome returns the PinchBot data directory.
// Priority: $PINCHBOT_HOME > $PICOCLAW_HOME > exe_dir/.openclaw
func GetPicoclawHome() string {
	return config.GetPinchBotHome()
}

func GetConfigPath() string {
	return config.GetConfigPath()
}

func LoadConfig() (*config.Config, error) {
	return config.LoadOrInitConfig(GetConfigPath())
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
