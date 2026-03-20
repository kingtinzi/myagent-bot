package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetManifestURLPrefersPinchBotEnvButFallsBackToLegacyEnv(t *testing.T) {
	prevBuildManifestURL := BuildManifestURL
	t.Cleanup(func() { BuildManifestURL = prevBuildManifestURL })
	BuildManifestURL = "https://build.example.com/manifest.json"

	t.Setenv("PINCHBOT_UPDATE_MANIFEST_URL", "https://pinchbot.example.com/manifest.json")
	t.Setenv("OPENCLAW_UPDATE_MANIFEST_URL", "https://openclaw.example.com/manifest.json")

	if got := getManifestURL(); got != "https://pinchbot.example.com/manifest.json" {
		t.Fatalf("getManifestURL() = %q, want pinchbot env value", got)
	}

	t.Setenv("PINCHBOT_UPDATE_MANIFEST_URL", "")
	if got := getManifestURL(); got != "https://openclaw.example.com/manifest.json" {
		t.Fatalf("getManifestURL() = %q, want legacy env fallback", got)
	}

	t.Setenv("OPENCLAW_UPDATE_MANIFEST_URL", "")
	if got := getManifestURL(); got != BuildManifestURL {
		t.Fatalf("getManifestURL() = %q, want build-time manifest url %q", got, BuildManifestURL)
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

func TestDownloadUpdateRejectsMismatchedSHA256(t *testing.T) {
	t.Setenv("PINCHBOT_PENDING_DIR", t.TempDir())
	payload := []byte("signed-update-payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	manifest := &UpdateManifest{
		Version:   "v1.2.3",
		URL:       server.URL + "/PinchBot.zip",
		ZipFolder: "PinchBot-1.2.3",
		SHA256:    strings.Repeat("0", sha256.Size*2),
	}

	zipPath, err := DownloadUpdate(context.Background(), manifest)
	if err == nil {
		t.Fatalf("DownloadUpdate() error = nil, want checksum validation failure")
	}
	if zipPath != "" {
		t.Fatalf("DownloadUpdate() zipPath = %q, want empty path on checksum failure", zipPath)
	}
}

func TestDownloadUpdateAcceptsMatchingSHA256(t *testing.T) {
	pendingDir := t.TempDir()
	t.Setenv("PINCHBOT_PENDING_DIR", pendingDir)
	payload := []byte("signed-update-payload")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	manifest := &UpdateManifest{
		Version:   "v1.2.3",
		URL:       server.URL + "/PinchBot.zip",
		ZipFolder: "PinchBot-1.2.3",
		SHA256:    hex.EncodeToString(sum[:]),
	}

	zipPath, err := DownloadUpdate(context.Background(), manifest)
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	if _, statErr := os.Stat(zipPath); statErr != nil {
		t.Fatalf("downloaded zip missing at %q: %v", zipPath, statErr)
	}
}

func TestDownloadUpdateRequiresSHA256(t *testing.T) {
	t.Setenv("PINCHBOT_PENDING_DIR", t.TempDir())
	manifest := &UpdateManifest{
		Version:   "v1.2.3",
		URL:       "https://example.com/update.zip",
		ZipFolder: "PinchBot-1.2.3",
	}

	zipPath, err := DownloadUpdate(context.Background(), manifest)
	if err == nil {
		t.Fatal("DownloadUpdate() error = nil, want missing sha256 failure")
	}
	if zipPath != "" {
		t.Fatalf("zipPath = %q, want empty path when sha256 is missing", zipPath)
	}
}

func TestValidateUpdateTransportURLRejectsInsecureRemoteURL(t *testing.T) {
	if err := validateUpdateTransportURL("http://example.com/update.zip"); err == nil {
		t.Fatal("validateUpdateTransportURL() error = nil, want insecure remote url rejection")
	}
}

func TestValidateUpdateTransportURLRejectsMissingHostname(t *testing.T) {
	if err := validateUpdateTransportURL("https:///update.zip"); err == nil {
		t.Fatal("validateUpdateTransportURL() error = nil, want missing hostname rejection")
	}
}

func TestValidateUpdateTransportURLAllowsLoopbackHTTP(t *testing.T) {
	if err := validateUpdateTransportURL("http://127.0.0.1/update.zip"); err != nil {
		t.Fatalf("validateUpdateTransportURL() error = %v, want local loopback url allowed", err)
	}
}

func TestRunApplyScriptAndExitEscapesPowerShellSingleQuotes(t *testing.T) {
	pendingDir := t.TempDir()
	t.Setenv("PINCHBOT_PENDING_DIR", pendingDir)
	metaBytes, err := json.Marshal(pendingMeta{
		ZipPath:    `C:\temp\Pinch'Bot-update.zip`,
		ZipFolder:  `Pinch'Bot-1.2.3`,
		InstallDir: `C:\Program Files\Pinch'Bot`,
	})
	if err != nil {
		t.Fatalf("Marshal(pendingMeta) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pendingDir, pendingMetaFile), metaBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(pending meta) error = %v", err)
	}

	RunApplyScriptAndExit()

	scriptPath := filepath.Join(pendingDir, "apply_update.ps1")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", scriptPath, err)
	}
	script := string(scriptBytes)
	for _, want := range []string{
		`$zip = 'C:\temp\Pinch''Bot-update.zip'`,
		`$dst = 'C:\Program Files\Pinch''Bot'`,
		`$inner = 'Pinch''Bot-1.2.3'`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("apply script missing escaped literal %q\nscript:\n%s", want, script)
		}
	}
}
