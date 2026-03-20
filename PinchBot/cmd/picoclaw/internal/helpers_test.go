package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "custom", "pinchbot-home")
	t.Setenv("PINCHBOT_HOME", home)

	got := GetConfigPath()
	want := filepath.Join(home, "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithPICOCLAW_HOME(t *testing.T) {
	home := filepath.Join(t.TempDir(), "custom", "picoclaw")
	t.Setenv("PINCHBOT_HOME", "")
	t.Setenv("PINCHBOT_CONFIG", "")
	t.Setenv("PICOCLAW_HOME", home)

	got := GetConfigPath()
	want := filepath.Join(home, "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithPINCHBOT_CONFIG(t *testing.T) {
	home := filepath.Join(t.TempDir(), "custom", "pinchbot-home")
	cfgPath := filepath.Join(t.TempDir(), "custom", "pinchbot-config.json")
	t.Setenv("PINCHBOT_CONFIG", cfgPath)
	t.Setenv("PINCHBOT_HOME", home)

	got := GetConfigPath()
	want := filepath.Clean(cfgPath)

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithPICOCLAW_CONFIG(t *testing.T) {
	home := filepath.Join(t.TempDir(), "custom", "picoclaw")
	cfgPath := filepath.Join(t.TempDir(), "custom", "config.json")
	t.Setenv("PINCHBOT_HOME", "")
	t.Setenv("PINCHBOT_CONFIG", "")
	t.Setenv("PICOCLAW_CONFIG", cfgPath)
	t.Setenv("PICOCLAW_HOME", home)

	got := GetConfigPath()
	want := filepath.Clean(cfgPath)

	assert.Equal(t, want, got)
}

func TestFormatVersion_NoGitCommit(t *testing.T) {
	oldVersion, oldGit := version, gitCommit
	t.Cleanup(func() { version, gitCommit = oldVersion, oldGit })

	version = "1.2.3"
	gitCommit = ""

	assert.Equal(t, "1.2.3", FormatVersion())
}

func TestFormatVersion_WithGitCommit(t *testing.T) {
	oldVersion, oldGit := version, gitCommit
	t.Cleanup(func() { version, gitCommit = oldVersion, oldGit })

	version = "1.2.3"
	gitCommit = "abc123"

	assert.Equal(t, "1.2.3 (git: abc123)", FormatVersion())
}

func TestFormatBuildInfo_UsesBuildTimeAndGoVersion_WhenSet(t *testing.T) {
	oldBuildTime, oldGoVersion := buildTime, goVersion
	t.Cleanup(func() { buildTime, goVersion = oldBuildTime, oldGoVersion })

	buildTime = "2026-02-20T00:00:00Z"
	goVersion = "go1.23.0"

	build, goVer := FormatBuildInfo()

	assert.Equal(t, buildTime, build)
	assert.Equal(t, goVersion, goVer)
}

func TestFormatBuildInfo_EmptyBuildTime_ReturnsEmptyBuild(t *testing.T) {
	oldBuildTime, oldGoVersion := buildTime, goVersion
	t.Cleanup(func() { buildTime, goVersion = oldBuildTime, oldGoVersion })

	buildTime = ""
	goVersion = "go1.23.0"

	build, goVer := FormatBuildInfo()

	assert.Empty(t, build)
	assert.Equal(t, goVersion, goVer)
}

func TestFormatBuildInfo_EmptyGoVersion_FallsBackToRuntimeVersion(t *testing.T) {
	oldBuildTime, oldGoVersion := buildTime, goVersion
	t.Cleanup(func() { buildTime, goVersion = oldBuildTime, oldGoVersion })

	buildTime = "x"
	goVersion = ""

	build, goVer := FormatBuildInfo()

	assert.Equal(t, "x", build)
	assert.Equal(t, runtime.Version(), goVer)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	t.Setenv("PINCHBOT_HOME", "")
	t.Setenv("PICOCLAW_HOME", "")
	t.Setenv("PINCHBOT_CONFIG", "")
	t.Setenv("PICOCLAW_CONFIG", "")

	got := GetConfigPath()
	require.True(
		t,
		strings.Contains(strings.ToLower(got), strings.ToLower(filepath.Join(".openclaw", "config.json"))),
		"GetConfigPath() = %q, want path containing %q",
		got,
		filepath.Join(".openclaw", "config.json"),
	)
}

func TestGetVersion(t *testing.T) {
	assert.Equal(t, "dev", GetVersion())
}

func TestGetConfigPath_WithEnv(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "tmp", "custom", "config.json")
	t.Setenv("PINCHBOT_CONFIG", cfgPath)

	got := GetConfigPath()
	want := filepath.Clean(cfgPath)

	assert.Equal(t, want, got)
}
