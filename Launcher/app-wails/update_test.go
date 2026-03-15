package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetManifestURLPrefersPinchBotEnvButFallsBackToLegacyEnv(t *testing.T) {
	t.Setenv("PINCHBOT_UPDATE_MANIFEST_URL", "https://pinchbot.example.com/manifest.json")
	t.Setenv("OPENCLAW_UPDATE_MANIFEST_URL", "https://openclaw.example.com/manifest.json")

	if got := getManifestURL(); got != "https://pinchbot.example.com/manifest.json" {
		t.Fatalf("getManifestURL() = %q, want pinchbot env value", got)
	}

	t.Setenv("PINCHBOT_UPDATE_MANIFEST_URL", "")
	if got := getManifestURL(); got != "https://openclaw.example.com/manifest.json" {
		t.Fatalf("getManifestURL() = %q, want legacy env fallback", got)
	}
}

func TestGetPendingDirPrefersPinchBotPathButReusesLegacyPendingFolder(t *testing.T) {
	localAppData := t.TempDir()
	legacyDir := filepath.Join(localAppData, "OpenClaw", "pending")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", legacyDir, err)
	}

	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("TEMP", "")
	t.Setenv("PINCHBOT_PENDING_DIR", "")
	t.Setenv("OPENCLAW_PENDING_DIR", "")

	got, err := getPendingDir()
	if err != nil {
		t.Fatalf("getPendingDir() error = %v", err)
	}
	if got != legacyDir {
		t.Fatalf("getPendingDir() = %q, want legacy dir %q when it already exists", got, legacyDir)
	}
}

func TestGetPendingDirUsesPinchBotDefaultWhenNoLegacyStateExists(t *testing.T) {
	localAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("TEMP", "")
	t.Setenv("PINCHBOT_PENDING_DIR", "")
	t.Setenv("OPENCLAW_PENDING_DIR", "")

	got, err := getPendingDir()
	if err != nil {
		t.Fatalf("getPendingDir() error = %v", err)
	}
	want := filepath.Join(localAppData, "PinchBot", "pending")
	if got != want {
		t.Fatalf("getPendingDir() = %q, want %q", got, want)
	}
}
