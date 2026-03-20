package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"strconv"
)

func TestNodeHost_ManagedLifecycle_RestartOnCrash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	repoRoot := repoRootFromCwd(t)
	extensionsRoot := filepath.Join(repoRoot, "extensions")

	discovered, err := DiscoverEnabled(extensionsRoot, []string{"echo-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered plugin, got %d (%+v)", len(discovered), discovered)
	}

	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	managed, catalog, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:    workspace,
		Sandboxed:    false,
		Discovered:   discovered,
		HostDir:      "", // default resolution
		NodeBinary:   "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !catalogHasTool(catalog, "echo_fixture") {
		t.Fatalf("expected echo_fixture tool in catalog, got: %+v", catalog)
	}

	out, err := managed.Execute(ctx, "echo-fixture", "echo_fixture", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Execute 1: %v", err)
	}
	if !strings.Contains(out, `"hello"`) {
		t.Fatalf("unexpected output: %s", out)
	}

	// Simulate a hard crash of the host process. The next Execute should restart + init + succeed.
	managed.mu.Lock()
	old := managed.node
	if old != nil && old.cmd != nil && old.cmd.Process != nil {
		_ = old.cmd.Process.Kill()
	}
	if old != nil {
		_ = old.Close()
	}
	managed.node = nil
	managed.mu.Unlock()

	out2, err := managed.Execute(ctx, "echo-fixture", "echo_fixture", map[string]any{"message": "world"})
	if err != nil {
		t.Fatalf("Execute 2 after crash: %v", err)
	}
	if !strings.Contains(out2, `"world"`) {
		t.Fatalf("unexpected output after restart: %s", out2)
	}
}

func catalogHasTool(catalog []CatalogTool, toolName string) bool {
	for _, ct := range catalog {
		if ct.Name == toolName {
			return true
		}
	}
	return false
}

func repoRootFromCwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// We are in PinchBot/pkg/plugins; repo root for PinchBot is two levels up.
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func hasPluginHostNodeModules(t *testing.T) bool {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// cwd = PinchBot/pkg/plugins → assets = PinchBot/pkg/plugins/assets
	assetsDir := filepath.Join(cwd, "assets")
	if _, err := os.Stat(filepath.Join(assetsDir, "node_modules", "jiti")); err != nil {
		t.Skipf("plugin host deps not installed (%s); run: cd %s && npm ci", err, assetsDir)
		return false
	}
	return true
}

func hasUsableNode18(t *testing.T) bool {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH")
		return false
	}
	out, err := exec.Command(nodePath, "-p", "process.versions.node").CombinedOutput()
	if err != nil {
		t.Skipf("node version check failed: %v (%s)", err, strings.TrimSpace(string(out)))
		return false
	}
	v := strings.TrimSpace(string(out))
	majorStr := strings.SplitN(v, ".", 2)[0]
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		t.Skipf("unexpected node version: %q", v)
		return false
	}
	if major < 18 {
		t.Skipf("node >= 18 required, got %s", v)
		return false
	}
	return true
}

