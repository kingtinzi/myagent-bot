package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilesLoadsMissingValuesWithoutOverridingExistingEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, "platform.env")
	if err := os.WriteFile(envPath, []byte("PLATFORM_SUPABASE_URL=https://file.example.com\nPLATFORM_ADDR=:29999\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("PLATFORM_ADDR", ":18888")
	if err := LoadEnvFiles(envPath); err != nil {
		t.Fatalf("LoadEnvFiles() error = %v", err)
	}

	if got := os.Getenv("PLATFORM_SUPABASE_URL"); got != "https://file.example.com" {
		t.Fatalf("PLATFORM_SUPABASE_URL = %q, want %q", got, "https://file.example.com")
	}
	if got := os.Getenv("PLATFORM_ADDR"); got != ":18888" {
		t.Fatalf("PLATFORM_ADDR = %q, want existing env to win", got)
	}
}

func TestLoadPlatformEnvOnlyUsesExplicitLiveFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	examplePath := filepath.Join(configDir, "platform.example.env")
	if err := os.WriteFile(examplePath, []byte("PLATFORM_SUPABASE_URL=https://example-only.invalid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(example) error = %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(wd); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	}()

	t.Setenv("PLATFORM_SUPABASE_URL", "")
	if err := os.Unsetenv("PLATFORM_SUPABASE_URL"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}

	if err := LoadPlatformEnv(); err != nil {
		t.Fatalf("LoadPlatformEnv() error = %v", err)
	}
	if got := os.Getenv("PLATFORM_SUPABASE_URL"); got != "" {
		t.Fatalf("PLATFORM_SUPABASE_URL = %q, want empty when only example file exists", got)
	}

	livePath := filepath.Join(configDir, "platform.env")
	if err := os.WriteFile(livePath, []byte("PLATFORM_SUPABASE_URL=https://live.example.com\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(live) error = %v", err)
	}

	if err := LoadPlatformEnv(); err != nil {
		t.Fatalf("LoadPlatformEnv() second call error = %v", err)
	}
	if got := os.Getenv("PLATFORM_SUPABASE_URL"); got != "https://live.example.com" {
		t.Fatalf("PLATFORM_SUPABASE_URL = %q, want %q", got, "https://live.example.com")
	}
}
