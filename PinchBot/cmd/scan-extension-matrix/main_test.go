package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "demo-ext"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo-ext", "openclaw.plugin.json"), []byte(`{"id":"demo","name":"Demo","configSchema":{"type":"object"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo-ext", "index.ts"), []byte(`api.registerTool(() => ({}));`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].id != "demo" {
		t.Fatalf("got %+v", rows)
	}
	if rows[0].indexSrc == "" {
		t.Fatal("expected index.ts read")
	}
}
