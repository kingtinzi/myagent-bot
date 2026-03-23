package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	manifest := `{"id":"lobster","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(lob, manifestName), []byte(manifest), 0o644); err != nil {
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

func TestDiscoverEnabled_EnabledPluginMissingConfigSchema(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "badext")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, manifestName), []byte(`{"id":"badext"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DiscoverEnabled(root, []string{"badext"})
	if err == nil {
		t.Fatal("expected error when enabled plugin manifest lacks configSchema")
	}
}

func extensionsDirFromPkgPlugins(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..", "extensions"))
}

func TestDiscoverEnabled_TwoBundledFixtures(t *testing.T) {
	ext := extensionsDirFromPkgPlugins(t)
	if _, err := os.Stat(filepath.Join(ext, "fixture-second", manifestName)); err != nil {
		t.Fatalf("missing bundled fixture-second: %v", err)
	}
	found, err := DiscoverEnabled(ext, []string{"echo-fixture", "fixture-second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("want 2 plugins, got %+v", found)
	}
	ids := []string{found[0].ID, found[1].ID}
	sort.Strings(ids)
	if ids[0] != "echo-fixture" || ids[1] != "fixture-second" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestDiscoverEnabled_TwoValidPlugins(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"alpha", "beta"} {
		d := filepath.Join(root, id)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, id)
		if err := os.WriteFile(filepath.Join(d, manifestName), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := DiscoverEnabled(root, []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("want 2 plugins, got %+v", found)
	}
	ids := []string{found[0].ID, found[1].ID}
	sort.Strings(ids)
	if ids[0] != "alpha" || ids[1] != "beta" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestDiscoverEnabled_UnrelatedBadManifestSkipped(t *testing.T) {
	root := t.TempDir()
	junk := filepath.Join(root, "junk")
	if err := os.MkdirAll(junk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(junk, manifestName), []byte(`{"id":"junk"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := DiscoverEnabled(root, []string{"lobster"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("expected no match for enabled lobster, got %+v", found)
	}
}
