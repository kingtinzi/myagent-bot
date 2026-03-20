package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlatformEnvFromPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "platform.env")
	if err := os.WriteFile(p, []byte("PICOCLAW_PLATFORM_API_BASE_URL=http://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOCLAW_PLATFORM_API_BASE_URL", "")
	if err := LoadPlatformEnvFromPath(p); err != nil {
		t.Fatalf("LoadPlatformEnvFromPath: %v", err)
	}
	if got := os.Getenv("PICOCLAW_PLATFORM_API_BASE_URL"); got != "http://example.com" {
		t.Fatalf("env = %q, want http://example.com", got)
	}
}

func TestLoadPlatformEnvFromPathDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "platform.env")
	if err := os.WriteFile(p, []byte("PICOCLAW_PLATFORM_API_BASE_URL=http://from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOCLAW_PLATFORM_API_BASE_URL", "http://already-set")
	if err := LoadPlatformEnvFromPath(p); err != nil {
		t.Fatalf("LoadPlatformEnvFromPath: %v", err)
	}
	if got := os.Getenv("PICOCLAW_PLATFORM_API_BASE_URL"); got != "http://already-set" {
		t.Fatalf("env = %q, want preserved", got)
	}
}

func TestLoadPlatformEnvFromPathMissingOK(t *testing.T) {
	if err := LoadPlatformEnvFromPath(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
