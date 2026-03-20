package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExtensionsDir(t *testing.T) {
	ws := t.TempDir()
	got, err := ResolveExtensionsDir(ws, "ext")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(ws, "ext")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	abs := filepath.Join(ws, "abs")
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}
	got2, err := ResolveExtensionsDir(ws, abs)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != abs {
		t.Fatalf("absolute: got %q want %q", got2, abs)
	}
}

func TestDiscoverEnabled(t *testing.T) {
	root := t.TempDir()
	lob := filepath.Join(root, "lobster")
	if err := os.MkdirAll(lob, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lob, manifestName), []byte(`{"id":"lobster"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// skipped: hidden and underscore prefixes
	_ = os.MkdirAll(filepath.Join(root, ".hidden"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "_skip"), 0o755)

	found, err := DiscoverEnabled(root, []string{"lobster"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != "lobster" {
		t.Fatalf("discover: %+v", found)
	}

	found2, err := DiscoverEnabled(root, []string{"LOBSter"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found2) != 1 {
		t.Fatalf("case insensitive: %+v", found2)
	}

	found3, err := DiscoverEnabled(root, []string{"other"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found3) != 0 {
		t.Fatalf("expected empty, got %+v", found3)
	}
}
