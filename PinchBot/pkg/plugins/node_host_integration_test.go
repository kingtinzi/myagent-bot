package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/tools"
)

func providerSnapEmpty(s PluginProviderSnapshots) bool {
	return len(s.Text) == 0 && len(s.Speech) == 0 && len(s.MediaUnderstanding) == 0 &&
		len(s.ImageGeneration) == 0 && len(s.WebSearch) == 0
}

func TestNodeHost_TwoBundledFixtures_LoadTogether(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(extensionsRoot, "fixture-second", manifestName)); err != nil {
		t.Fatalf("fixture-second missing: %v", err)
	}

	discovered, err := DiscoverEnabled(extensionsRoot, []string{"echo-fixture", "fixture-second"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered plugins, got %d (%+v)", len(discovered), discovered)
	}

	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, catalog, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "echo-fixture") || !diagnosticOK(diags, "fixture-second") {
		t.Fatalf("expected both plugins ok in diagnostics, got: %+v", diags)
	}
	if !catalogHasTool(catalog, "echo_fixture") || !catalogHasTool(catalog, "fixture_second_echo") {
		t.Fatalf("expected both tools in catalog, got: %+v", catalog)
	}
}

func TestNodeHost_ThreePlugins_OneRegisterFails_OthersLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	writeEchoPlugin(t, extRoot, "pa", "good-a", "tool_a")
	writeEchoPlugin(t, extRoot, "pb", "good-b", "tool_b")
	writeBrokenRegisterPlugin(t, extRoot, "pc", "bad-reg")

	discovered, err := DiscoverEnabled(extRoot, []string{"good-a", "good-b", "bad-reg"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	if len(discovered) != 3 {
		t.Fatalf("want 3 discovered, got %+v", discovered)
	}

	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, catalog, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "good-a") || !diagnosticOK(diags, "good-b") {
		t.Fatalf("expected good-a and good-b ok, got: %+v", diags)
	}
	var bad *PluginInitDiagnostic
	for i := range diags {
		if diags[i].PluginID == "bad-reg" {
			bad = &diags[i]
			break
		}
	}
	if bad == nil || bad.OK || !strings.Contains(bad.Error, "intentional fixture failure") {
		t.Fatalf("expected bad-reg failure with intentional error, got: %+v", diags)
	}
	if !catalogHasTool(catalog, "tool_a") || !catalogHasTool(catalog, "tool_b") {
		t.Fatalf("expected tool_a and tool_b in catalog, got: %+v", catalog)
	}
	if catalogHasTool(catalog, "bad_tool") {
		t.Fatal("did not expect tools from failed plugin")
	}
}

func writeEchoPlugin(t *testing.T, extRoot, dirName, pluginID, toolName string) {
	t.Helper()
	d := filepath.Join(extRoot, dirName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, pluginID)
	if err := os.WriteFile(filepath.Join(d, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := fmt.Sprintf(`import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: %q,
  name: "t",
  description: "t",
  register(api: any) {
    api.registerTool(
      () => ({
        name: %q,
        description: "t",
        parameters: {
          type: "object",
          properties: { message: { type: "string" } },
          required: ["message"],
          additionalProperties: true,
        },
        async execute(_id: string, params: Record<string, unknown>) {
          return { content: [{ type: "text", text: JSON.stringify({ ok: true, params }, null, 2) }] };
        },
      }),
      {},
    );
  },
});
`, pluginID, toolName)
	if err := os.WriteFile(filepath.Join(d, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeBrokenRegisterPlugin(t *testing.T, extRoot, dirName, pluginID string) {
	t.Helper()
	d := filepath.Join(extRoot, dirName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, pluginID)
	if err := os.WriteFile(filepath.Join(d, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `export default { register() { throw new Error("intentional fixture failure"); } };`
	if err := os.WriteFile(filepath.Join(d, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mode: "block" — before_tool_call returns { blocked: true }; "rewrite" — replaces args.message.
func writeBeforeToolCallPlugin(t *testing.T, extRoot, dirName, pluginID, toolName, mode string) {
	t.Helper()
	d := filepath.Join(extRoot, dirName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, pluginID)
	if err := os.WriteFile(filepath.Join(d, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var hookBody string
	switch mode {
	case "block":
		hookBody = `api.on("before_tool_call", () => ({ blocked: true, error: "stop-here" }));`
	case "rewrite":
		hookBody = `api.on("before_tool_call", () => ({ args: { message: "from-hook" } }));`
	default:
		t.Fatalf("unknown mode %q", mode)
	}
	ts := fmt.Sprintf(`import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: %q,
  name: "t",
  description: "t",
  register(api: any) {
    %s
    api.registerTool(
      () => ({
        name: %q,
        description: "t",
        parameters: {
          type: "object",
          properties: { message: { type: "string" } },
          required: ["message"],
          additionalProperties: true,
        },
        async execute(_id: string, params: Record<string, unknown>) {
          return { content: [{ type: "text", text: JSON.stringify({ ok: true, params }, null, 2) }] };
        },
      }),
      {},
    );
  },
});
`, pluginID, hookBody, toolName)
	if err := os.WriteFile(filepath.Join(d, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNodeHost_BeforeToolCall_BlocksExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	writeBeforeToolCallPlugin(t, extRoot, "p1", "btc-block", "btc_tool", "block")

	discovered, err := DiscoverEnabled(extRoot, []string{"btc-block"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "btc-block") {
		t.Fatalf("expected btc-block ok, got: %+v", diags)
	}

	execCtx := tools.WithAgentID(tools.WithToolContext(context.Background(), "telegram", "c1"), "main")
	_, err = managed.Execute(execCtx, "btc-block", "btc_tool", map[string]any{"message": "hi"})
	if err == nil {
		t.Fatal("expected blocked error")
	}
	if !strings.Contains(err.Error(), "PINCHBOT_TOOL_BLOCKED:") || !strings.Contains(err.Error(), "stop-here") {
		t.Fatalf("expected block sentinel in error, got: %v", err)
	}
}

func TestNodeHost_BeforeToolCall_RewritesArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	writeBeforeToolCallPlugin(t, extRoot, "p1", "btc-rewrite", "btw_tool", "rewrite")

	discovered, err := DiscoverEnabled(extRoot, []string{"btc-rewrite"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "btc-rewrite") {
		t.Fatalf("expected btc-rewrite ok, got: %+v", diags)
	}

	execCtx := tools.WithAgentID(tools.WithToolContext(context.Background(), "cli", "direct"), "sub-1")
	out, err := managed.Execute(execCtx, "btc-rewrite", "btw_tool", map[string]any{"message": "original"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "from-hook") {
		t.Fatalf("expected rewritten message in output, got: %q", out)
	}
	if strings.Contains(out, "original") {
		t.Fatalf("did not expect original args in output, got: %q", out)
	}
}

func writeAfterToolCallPlugin(t *testing.T, extRoot, dirName, pluginID, toolName string) {
	t.Helper()
	d := filepath.Join(extRoot, dirName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, pluginID)
	if err := os.WriteFile(filepath.Join(d, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := fmt.Sprintf(`import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: %q,
  name: "t",
  description: "t",
  register(api: any) {
    api.on("after_tool_call", () => {
      throw new Error("after-ignored");
    });
    api.registerTool(
      () => ({
        name: %q,
        description: "t",
        parameters: {
          type: "object",
          properties: { message: { type: "string" } },
          required: ["message"],
          additionalProperties: true,
        },
        async execute(_id: string, params: Record<string, unknown>) {
          return { content: [{ type: "text", text: JSON.stringify({ ok: true, params }, null, 2) }] };
        },
      }),
      {},
    );
  },
});
`, pluginID, toolName)
	if err := os.WriteFile(filepath.Join(d, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNodeHost_ResolvePath_RelativeToPluginRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "resolve-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":%q,"configSchema":{"type":"object","properties":{}}}`, "resolve-fixture")
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "resolve-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerTool(
      () => ({
        name: "rp_tool",
        description: "resolvePath smoke",
        parameters: {
          type: "object",
          properties: { rel: { type: "string" } },
          required: ["rel"],
        },
        async execute(_id: string, params: Record<string, unknown>) {
          const rel = String(params.rel ?? "");
          const abs = api.resolvePath(rel);
          return { content: [{ type: "text", text: abs }] };
        },
      }),
      {},
    );
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"resolve-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "resolve-fixture") {
		t.Fatalf("expected resolve-fixture ok, got: %+v", diags)
	}

	wantAbs := filepath.Join(pluginDir, "nested", "file.txt")
	out, err := managed.Execute(context.Background(), "resolve-fixture", "rp_tool", map[string]any{"rel": "nested/file.txt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Node returns platform-normalized absolute path as text
	if !strings.Contains(out, filepath.Clean(wantAbs)) && !strings.Contains(out, wantAbs) {
		t.Fatalf("expected resolved path containing %q, got: %q", wantAbs, out)
	}
}

func TestNodeHost_RegisterHttpRoute_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "http-route-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"http-route-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "http-route-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerHttpRoute({
      method: "GET",
      path: "/pinchbot-http-route-decl",
      handler: async () => ({ status: 200, body: "hello", headers: { "x-test": "1" } }),
    });
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"http-route-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "http-route-fixture") {
		t.Fatalf("expected http-route-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 1 {
		t.Fatalf("expected 1 http route, got %+v", routes)
	}
	if len(cmds) != 0 {
		t.Fatalf("expected no cli commands, got %+v", cmds)
	}
	if len(gwm) != 0 {
		t.Fatalf("expected no gateway methods, got %+v", gwm)
	}
	if len(rsv) != 0 {
		t.Fatalf("expected no registered services, got %+v", rsv)
	}
	if len(rcli) != 0 {
		t.Fatalf("expected no registerCli rows, got %+v", rcli)
	}
	if !providerSnapEmpty(rpv) {
		t.Fatalf("expected no registered providers, got %+v", rpv)
	}
	if routes[0].PluginID != "http-route-fixture" || routes[0].Method != "GET" || routes[0].Path != "/pinchbot-http-route-decl" {
		t.Fatalf("unexpected route: %+v", routes[0])
	}

	st := BuildPluginInitStatus(nil, diags, routes, nil, nil, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{})
	if len(st) != 1 || len(st[0].HTTPRoutes) != 1 || st[0].HTTPRoutes[0] != "GET /pinchbot-http-route-decl" {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	res, err := managed.HTTPRoute(ctx2, "http-route-fixture", "GET", "/pinchbot-http-route-decl", "", nil, nil)
	if err != nil {
		t.Fatalf("HTTPRoute: %v", err)
	}
	if res == nil || res.Status != 200 || res.Body != "hello" {
		t.Fatalf("got %#v", res)
	}
	if res.Headers["x-test"] != "1" {
		t.Fatalf("headers: %+v", res.Headers)
	}
}

func TestNodeHost_RegisterCommand_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "cli-cmd-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"cli-cmd-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "cli-cmd-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerCommand({ name: "ping", description: "say hi" });
    api.registerCommand({ name: "pong" });
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"cli-cmd-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "cli-cmd-fixture") {
		t.Fatalf("expected cli-cmd-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 {
		t.Fatalf("expected no http routes, got %+v", routes)
	}
	if len(gwm) != 0 {
		t.Fatalf("expected no gateway methods, got %+v", gwm)
	}
	if len(rsv) != 0 {
		t.Fatalf("expected no registered services, got %+v", rsv)
	}
	if len(rcli) != 0 {
		t.Fatalf("expected no registerCli rows, got %+v", rcli)
	}
	if !providerSnapEmpty(rpv) {
		t.Fatalf("expected no registered providers, got %+v", rpv)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 cli commands, got %+v", cmds)
	}
	if cmds[0].PluginID != "cli-cmd-fixture" || cmds[0].Name != "ping" || cmds[0].Description != "say hi" {
		t.Fatalf("unexpected cmd[0]: %+v", cmds[0])
	}
	if cmds[1].Name != "pong" || cmds[1].Description != "" {
		t.Fatalf("unexpected cmd[1]: %+v", cmds[1])
	}

	st := BuildPluginInitStatus(nil, diags, nil, cmds, nil, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{})
	if len(st) != 1 || len(st[0].CLICommands) != 2 {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}
	if st[0].CLICommands[0] != "ping — say hi" || st[0].CLICommands[1] != "pong" {
		t.Fatalf("cli command lines: %+v", st[0].CLICommands)
	}
}

func TestNodeHost_RegisterGatewayMethod_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "gw-method-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"gw-method-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "gw-method-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerGatewayMethod("fixture.alpha", async () => ({ ok: true }));
    api.registerGatewayMethod("fixture.beta", async () => ({ ok: true }));
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"gw-method-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "gw-method-fixture") {
		t.Fatalf("expected gw-method-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 || len(cmds) != 0 {
		t.Fatalf("expected only gateway methods, routes=%+v cmds=%+v", routes, cmds)
	}
	if len(rsv) != 0 {
		t.Fatalf("expected no registered services, got %+v", rsv)
	}
	if len(rcli) != 0 {
		t.Fatalf("expected no registerCli rows, got %+v", rcli)
	}
	if !providerSnapEmpty(rpv) {
		t.Fatalf("expected no registered providers, got %+v", rpv)
	}
	if len(gwm) != 2 {
		t.Fatalf("expected 2 gateway methods, got %+v", gwm)
	}
	if gwm[0].PluginID != "gw-method-fixture" || gwm[0].Method != "fixture.alpha" {
		t.Fatalf("unexpected gwm[0]: %+v", gwm[0])
	}
	if gwm[1].Method != "fixture.beta" {
		t.Fatalf("unexpected gwm[1]: %+v", gwm[1])
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, gwm, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{})
	if len(st) != 1 || len(st[0].GatewayMethods) != 2 {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}
	if st[0].GatewayMethods[0] != "fixture.alpha" || st[0].GatewayMethods[1] != "fixture.beta" {
		t.Fatalf("gateway_methods: %+v", st[0].GatewayMethods)
	}

	raw, err := managed.GatewayMethod(ctx, "gw-method-fixture", "fixture.alpha", map[string]any{"q": 1})
	if err != nil {
		t.Fatalf("GatewayMethod: %v", err)
	}
	var gm struct {
		Responded bool            `json:"responded"`
		GatewayOK bool            `json:"gatewayOk"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &gm); err != nil {
		t.Fatalf("decode gateway result: %v raw=%s", err, string(raw))
	}
	if !gm.Responded || !gm.GatewayOK {
		t.Fatalf("expected responded+gatewayOk: %+v", gm)
	}
	if !bytes.Contains(gm.Payload, []byte(`"ok"`)) {
		t.Fatalf("expected payload ok: %s", string(gm.Payload))
	}
}

func TestNodeHost_RegisterService_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "reg-svc-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"reg-svc-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "reg-svc-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerService({ id: "my.shared.svc", start: async () => {} });
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"reg-svc-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "reg-svc-fixture") {
		t.Fatalf("expected reg-svc-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 || len(cmds) != 0 || len(gwm) != 0 {
		t.Fatalf("unexpected snapshot: routes=%+v cmds=%+v gwm=%+v", routes, cmds, gwm)
	}
	if len(rcli) != 0 {
		t.Fatalf("expected no registerCli rows, got %+v", rcli)
	}
	if !providerSnapEmpty(rpv) {
		t.Fatalf("expected no registered providers, got %+v", rpv)
	}
	if len(rsv) != 1 || rsv[0].PluginID != "reg-svc-fixture" || rsv[0].ServiceID != "my.shared.svc" {
		t.Fatalf("registered services: %+v", rsv)
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, nil, rsv, nil, PluginProviderSnapshots{}, PluginInitExtras{})
	if len(st) != 1 || len(st[0].RegisteredServices) != 1 || st[0].RegisteredServices[0] != "my.shared.svc" {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}
}

func TestNodeHost_RegisterCli_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "reg-cli-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"reg-cli-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "reg-cli-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerCli(() => {}, { commands: ["shared-cli", "other-cmd"] });
    api.registerCli(async () => {});
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"reg-cli-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "reg-cli-fixture") {
		t.Fatalf("expected reg-cli-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 || len(cmds) != 0 || len(gwm) != 0 || len(rsv) != 0 {
		t.Fatalf("unexpected snapshot r=%+v c=%+v g=%+v s=%+v", routes, cmds, gwm, rsv)
	}
	if !providerSnapEmpty(rpv) {
		t.Fatalf("expected no registered providers, got %+v", rpv)
	}
	if len(rcli) != 2 {
		t.Fatalf("expected 2 registerCli rows, got %+v", rcli)
	}
	if len(rcli[0].Commands) != 2 || rcli[0].Commands[0] != "shared-cli" {
		t.Fatalf("rcli[0]: %+v", rcli[0])
	}
	if len(rcli[1].Commands) != 0 {
		t.Fatalf("rcli[1] should be empty commands: %+v", rcli[1])
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, nil, nil, rcli, PluginProviderSnapshots{}, PluginInitExtras{})
	if len(st) != 1 || len(st[0].RegisterCliCommands) != 3 {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}
	if st[0].RegisterCliCommands[0] != "shared-cli" || st[0].RegisterCliCommands[1] != "other-cmd" || st[0].RegisterCliCommands[2] != "registerCli" {
		t.Fatalf("register_cli_commands: %+v", st[0].RegisterCliCommands)
	}
}

func TestNodeHost_RegisterHookChannelBinding_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "hook-chan-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"hook-chan-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "hook-chan-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerHook(["before_model_resolve", "llm_input"], () => {}, {});
    api.registerHook("agent_end", () => {}, {});
    api.registerChannel({ plugin: { id: "demo-channel", meta: {}, capabilities: {} } } as any);
    api.registerInteractiveHandler({
      channel: "telegram",
      namespace: "ns1",
      handler: async () => ({}),
    } as any);
    api.onConversationBindingResolved(() => {});
    api.onConversationBindingResolved(async () => {});
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"hook-chan-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "hook-chan-fixture") {
		t.Fatalf("expected hook-chan-fixture ok, got: %+v", diags)
	}

	_, _, _, _, _, _, _, _, ex := managed.InitSnapshot()
	if len(ex.Hooks) != 2 {
		t.Fatalf("hooks: %+v", ex.Hooks)
	}
	if len(ex.Hooks[0].Events) != 2 || ex.Hooks[0].Events[0] != "before_model_resolve" || ex.Hooks[0].Events[1] != "llm_input" {
		t.Fatalf("hook[0] events: %+v", ex.Hooks[0])
	}
	if len(ex.Hooks[1].Events) != 1 || ex.Hooks[1].Events[0] != "agent_end" {
		t.Fatalf("hook[1] events: %+v", ex.Hooks[1])
	}
	if len(ex.Channels) != 1 || ex.Channels[0].ChannelID != "demo-channel" {
		t.Fatalf("channels: %+v", ex.Channels)
	}
	if len(ex.InteractiveHandlers) != 1 || ex.InteractiveHandlers[0].Channel != "telegram" || ex.InteractiveHandlers[0].Namespace != "ns1" {
		t.Fatalf("interactive: %+v", ex.InteractiveHandlers)
	}
	if len(ex.ConversationBindingListeners) != 2 {
		t.Fatalf("conv binding: %+v", ex.ConversationBindingListeners)
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, nil, nil, nil, PluginProviderSnapshots{}, ex)
	if len(st) != 1 {
		t.Fatalf("status len: %+v", st)
	}
	if len(st[0].RegisteredHooks) != 2 || st[0].RegisteredHooks[0] != "before_model_resolve, llm_input" || st[0].RegisteredHooks[1] != "agent_end" {
		t.Fatalf("status hooks: %+v", st[0].RegisteredHooks)
	}
	if len(st[0].RegisteredChannels) != 1 || st[0].RegisteredChannels[0] != "demo-channel" {
		t.Fatalf("status channels: %+v", st[0].RegisteredChannels)
	}
	if len(st[0].RegisteredInteractiveHandlers) != 1 || st[0].RegisteredInteractiveHandlers[0] != "telegram — ns1" {
		t.Fatalf("status interactive: %+v", st[0].RegisteredInteractiveHandlers)
	}
	if len(st[0].ConversationBindingResolvedListeners) != 2 {
		t.Fatalf("status conv: %+v", st[0].ConversationBindingResolvedListeners)
	}
}

func TestNodeHost_RegisterSiblingProviders_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "sibling-prov-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"sibling-prov-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "sibling-prov-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerSpeechProvider({ id: "sp1", label: "Speech", isConfigured: () => false, synthesize: async () => ({}) } as any);
    api.registerMediaUnderstandingProvider({ id: "mu1" } as any);
    api.registerImageGenerationProvider({
      id: "ig1",
      label: "ImgGen",
      capabilities: { generate: { default: true }, edit: { default: false } },
      generateImage: async () => ({ images: [] }),
    } as any);
    api.registerWebSearchProvider({
      id: "ws1",
      label: "WebS",
      hint: "",
      envVars: [],
      placeholder: "",
      signupUrl: "",
      credentialPath: "",
      getCredentialValue: () => null,
      setCredentialValue: () => {},
      createTool: () => null,
    } as any);
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"sibling-prov-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "sibling-prov-fixture") {
		t.Fatalf("expected sibling-prov-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 || len(cmds) != 0 || len(gwm) != 0 || len(rsv) != 0 || len(rcli) != 0 {
		t.Fatalf("unexpected snapshot")
	}
	if len(rpv.Text) != 0 {
		t.Fatalf("expected no text providers, got %+v", rpv.Text)
	}
	if len(rpv.Speech) != 1 || rpv.Speech[0].ID != "sp1" {
		t.Fatalf("speech: %+v", rpv.Speech)
	}
	if len(rpv.MediaUnderstanding) != 1 || rpv.MediaUnderstanding[0].ID != "mu1" {
		t.Fatalf("media: %+v", rpv.MediaUnderstanding)
	}
	if len(rpv.ImageGeneration) != 1 || rpv.ImageGeneration[0].Label != "ImgGen" {
		t.Fatalf("image gen: %+v", rpv.ImageGeneration)
	}
	if len(rpv.WebSearch) != 1 || rpv.WebSearch[0].ID != "ws1" {
		t.Fatalf("web search: %+v", rpv.WebSearch)
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, nil, nil, nil, rpv, PluginInitExtras{})
	if len(st[0].RegisteredSpeechProviders) != 1 || st[0].RegisteredSpeechProviders[0] != "sp1 — Speech" {
		t.Fatalf("status speech: %+v", st[0].RegisteredSpeechProviders)
	}
	if st[0].RegisteredMediaUnderstandingProviders[0] != "mu1" || st[0].RegisteredImageGenerationProviders[0] != "ig1 — ImgGen" || st[0].RegisteredWebSearchProviders[0] != "ws1 — WebS" {
		t.Fatalf("status siblings: %+v %+v %+v", st[0].RegisteredMediaUnderstandingProviders, st[0].RegisteredImageGenerationProviders, st[0].RegisteredWebSearchProviders)
	}
}

func TestNodeHost_RegisterProvider_InInitSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	pluginDir := filepath.Join(extRoot, "reg-prov-fixture")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"reg-prov-fixture","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := `import { definePluginEntry } from "openclaw/plugin-sdk/core";
export default definePluginEntry({
  id: "reg-prov-fixture",
  name: "t",
  description: "t",
  register(api: any) {
    api.registerProvider({
      id: "custom-llm",
      label: "Custom LLM",
      auth: [],
    });
  },
});
`
	if err := os.WriteFile(filepath.Join(pluginDir, "index.ts"), []byte(ts), 0o644); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverEnabled(extRoot, []string{"reg-prov-fixture"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "reg-prov-fixture") {
		t.Fatalf("expected reg-prov-fixture ok, got: %+v", diags)
	}

	_, _, routes, cmds, gwm, rsv, rcli, rpv, _ := managed.InitSnapshot()
	if len(routes) != 0 || len(cmds) != 0 || len(gwm) != 0 || len(rsv) != 0 || len(rcli) != 0 {
		t.Fatalf("unexpected snapshot")
	}
	if len(rpv.Text) != 1 || rpv.Text[0].PluginID != "reg-prov-fixture" || rpv.Text[0].ID != "custom-llm" || rpv.Text[0].Label != "Custom LLM" {
		t.Fatalf("registered providers: %+v", rpv)
	}

	st := BuildPluginInitStatus(nil, diags, nil, nil, nil, nil, nil, rpv, PluginInitExtras{})
	if len(st) != 1 || len(st[0].RegisteredProviders) != 1 {
		t.Fatalf("BuildPluginInitStatus: %+v", st)
	}
	if st[0].RegisteredProviders[0] != "custom-llm — Custom LLM" {
		t.Fatalf("registered_providers: %+v", st[0].RegisteredProviders)
	}
}

func TestNodeHost_AfterToolCall_HandlerErrorDoesNotFailExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	if !hasUsableNode18(t) {
		return
	}
	if !hasPluginHostNodeModules(t) {
		return
	}

	extRoot := t.TempDir()
	writeAfterToolCallPlugin(t, extRoot, "p1", "atc-hook", "atc_tool")

	discovered, err := DiscoverEnabled(extRoot, []string{"atc-hook"})
	if err != nil {
		t.Fatalf("DiscoverEnabled: %v", err)
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	managed, _, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "atc-hook") {
		t.Fatalf("expected atc-hook ok, got: %+v", diags)
	}

	out, err := managed.Execute(context.Background(), "atc-hook", "atc_tool", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("Execute: %v (after_tool_call errors must not fail the tool)", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected tool output, got: %q", out)
	}
}

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

	managed, catalog, diags, err := BootstrapManagedHost(ctx, ManagedHostOpts{
		Workspace:     workspace,
		Sandboxed:     false,
		Discovered:    discovered,
		HostDir:       "", // default resolution
		NodeBinary:    "node",
		MaxRecoveries: 2,
		RestartDelay:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BootstrapManagedHost: %v", err)
	}
	t.Cleanup(func() { _ = managed.Close() })

	if !diagnosticOK(diags, "echo-fixture") {
		t.Fatalf("expected echo-fixture init ok in diagnostics, got: %+v", diags)
	}

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

func diagnosticOK(diags []PluginInitDiagnostic, pluginID string) bool {
	for _, d := range diags {
		if d.PluginID == pluginID && d.OK {
			return true
		}
	}
	return false
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
