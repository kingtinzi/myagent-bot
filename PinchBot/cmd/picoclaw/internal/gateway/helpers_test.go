package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureWorkspaceBootstrapCreatesTemplatesForMissingWorkspace(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")

	if err := ensureWorkspaceBootstrap(workspace); err != nil {
		t.Fatalf("ensureWorkspaceBootstrap() error = %v", err)
	}

	for _, rel := range []string{"AGENTS.md", "USER.md"} {
		target := filepath.Join(workspace, rel)
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected workspace template %s to exist: %v", target, err)
		}
	}
}

func TestEnsureWorkspaceBootstrapLeavesExistingWorkspaceContent(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	markerPath := filepath.Join(workspace, "custom.txt")
	if err := os.WriteFile(markerPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ensureWorkspaceBootstrap(workspace); err != nil {
		t.Fatalf("ensureWorkspaceBootstrap() error = %v", err)
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "custom" {
		t.Fatalf("custom workspace file = %q, want %q", string(data), "custom")
	}
}
