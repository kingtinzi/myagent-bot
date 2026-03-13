package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceWorkingDirReturnsPackageRootForMacAppBundle(t *testing.T) {
	exePath := filepath.Join("tmp", "OpenClaw", "launcher-chat.app", "Contents", "MacOS", "platform-server")

	got := serviceWorkingDir(exePath)
	want := filepath.Join("tmp", "OpenClaw")
	if got != want {
		t.Fatalf("serviceWorkingDir() = %q, want %q", got, want)
	}
}

func TestServiceWorkingDirKeepsExecutableDirectoryOutsideAppBundle(t *testing.T) {
	exePath := filepath.Join("tmp", "OpenClaw", "platform-server.exe")

	got := serviceWorkingDir(exePath)
	want := filepath.Join("tmp", "OpenClaw")
	if got != want {
		t.Fatalf("serviceWorkingDir() = %q, want %q", got, want)
	}
}

func TestPlatformConfigPathUsesPackageRootConfigDirectory(t *testing.T) {
	exePath := filepath.Join("tmp", "OpenClaw", "launcher-chat.app", "Contents", "MacOS", "platform-server")

	got := platformConfigPath(exePath)
	want := filepath.Join("tmp", "OpenClaw", "config", "platform.env")
	if got != want {
		t.Fatalf("platformConfigPath() = %q, want %q", got, want)
	}
}

func TestDefaultSessionStoreDirUsesPinchBotHome(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv("PINCHBOT_HOME", homeDir)
	t.Setenv("PINCHBOT_CONFIG", filepath.Join("nested", "config.json"))

	got := defaultSessionStoreDir()
	want := homeDir
	if got != want {
		t.Fatalf("defaultSessionStoreDir() = %q, want %q", got, want)
	}
}

func TestServiceProcessEnvPinsCurrentPinchBotHome(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv("PINCHBOT_HOME", homeDir)

	env := serviceProcessEnvWithBase([]string{
		"PINCHBOT_HOME=C:\\stale-home",
		"FOO=bar",
	})

	foundHome := false
	for _, entry := range env {
		switch entry {
		case "PINCHBOT_HOME=" + homeDir:
			foundHome = true
		case "PINCHBOT_HOME=C:\\stale-home":
			t.Fatalf("serviceProcessEnvWithBase() kept stale PINCHBOT_HOME entry: %q", entry)
		}
	}
	if !foundHome {
		t.Fatalf("serviceProcessEnvWithBase() missing effective PINCHBOT_HOME=%q", homeDir)
	}
}

func TestHasLivePlatformConfigRequiresARealFile(t *testing.T) {
	exePath := filepath.Join("tmp", "OpenClaw", "launcher-chat.app", "Contents", "MacOS", "platform-server")

	if hasLivePlatformConfig(func(string) (fs.FileInfo, error) {
		return nil, errors.New("missing")
	}, exePath) {
		t.Fatal("expected missing config file to disable platform auto-start")
	}

	if hasLivePlatformConfig(func(string) (fs.FileInfo, error) {
		return fakeFileInfo{dir: true}, nil
	}, exePath) {
		t.Fatal("expected config directories to be rejected for platform auto-start")
	}

	if !hasLivePlatformConfig(func(string) (fs.FileInfo, error) {
		return fakeFileInfo{}, nil
	}, exePath) {
		t.Fatal("expected a real config file to enable platform auto-start")
	}
}

type fakeFileInfo struct {
	dir bool
}

func (f fakeFileInfo) Name() string      { return "platform.env" }
func (f fakeFileInfo) Size() int64       { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode { return 0 }
func (f fakeFileInfo) ModTime() time.Time {
	return time.Time{}
}
func (f fakeFileInfo) IsDir() bool      { return f.dir }
func (f fakeFileInfo) Sys() interface{} { return nil }
