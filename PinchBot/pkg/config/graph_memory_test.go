package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGraphMemoryConfigPath(t *testing.T) {
	t.Setenv("PINCHBOT_GRAPH_MEMORY_CONFIG", "")
	p := ResolveGraphMemoryConfigPath("/a/b/config.json")
	want := filepath.Join("/a/b", "config.graph-memory.json")
	if p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestLoadGraphMemorySidecar(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "config.json")
	gm := filepath.Join(dir, "config.graph-memory.json")
	if err := os.WriteFile(gm, []byte(`{"enabled":false,"dbPath":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGraphMemorySidecar(main)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil when enabled false, got %+v", got)
	}
	if err := os.WriteFile(gm, []byte(`{"enabled":true,"dbPath":"/data/gm.db"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = LoadGraphMemorySidecar(main)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Enabled {
		t.Fatalf("expected enabled sidecar, got %+v", got)
	}
	if got.Raw["dbPath"] != "/data/gm.db" {
		t.Fatalf("raw dbPath: %v", got.Raw["dbPath"])
	}
}
