package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// NodeHost runs pkg/plugins/assets/run.mjs and speaks one JSON object per line on stdin/stdout.
type NodeHost struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	out    *bufio.Scanner
	mu     sync.Mutex
	nextID atomic.Int64
}

type nodeRequest struct {
	ID     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type nodeResponse struct {
	ID    int64           `json:"id"`
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// CatalogTool describes a tool registered by the Node host.
type CatalogTool struct {
	PluginID    string         `json:"pluginId"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func resolvePluginAssetsDir(hostDirOverride string, workspace string) (string, error) {
	hostDirOverride = strings.TrimSpace(hostDirOverride)
	if hostDirOverride != "" {
		if filepath.IsAbs(hostDirOverride) {
			return filepath.Clean(hostDirOverride), nil
		}
		if workspace == "" {
			return "", errors.New("workspace required for relative host_dir")
		}
		return filepath.Abs(filepath.Join(workspace, hostDirOverride))
	}
	if dir, ok := sourceTreePluginAssetsDir(); ok {
		return dir, nil
	}
	if dir := executableAdjacentPluginHostDir(); dir != "" {
		return dir, nil
	}
	return "", errors.New("plugin host assets not found (set plugins.host_dir or ship a plugin-host directory next to the binary)")
}

// sourceTreePluginAssetsDir returns pkg/plugins/assets when running from a checkout (run.mjs present).
func sourceTreePluginAssetsDir() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(file), "assets")
	if _, err := os.Stat(filepath.Join(dir, "run.mjs")); err != nil {
		return "", false
	}
	return filepath.Clean(dir), true
}

// executableAdjacentPluginHostDir returns a directory next to the current executable containing run.mjs (release bundles).
func executableAdjacentPluginHostDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	dir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(dir, "plugin-host"),
		filepath.Join(dir, "pkg", "plugins", "assets"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "run.mjs")); err == nil {
			return filepath.Clean(c)
		}
	}
	return ""
}

// StartNodeHost spawns node with run.mjs under assetsDir (see pluginAssetsDir).
// The child is not tied to a context: the process runs until Close() so it survives past short-lived init contexts.
func StartNodeHost(nodeBinary, hostDirOverride, workspace string) (*NodeHost, error) {
	if strings.TrimSpace(nodeBinary) == "" {
		nodeBinary = "node"
	}
	assetsDir, err := resolvePluginAssetsDir(hostDirOverride, workspace)
	if err != nil {
		return nil, err
	}
	script := filepath.Join(assetsDir, "run.mjs")
	if _, err := os.Stat(script); err != nil {
		return nil, fmt.Errorf("plugin host script: %w", err)
	}

	cmd := exec.Command(nodeBinary, script)
	cmd.Dir = assetsDir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard // errors surface via JSON responses; avoid noisy logs unless debugging

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start node host: %w", err)
	}

	out := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024)
	out.Buffer(buf, 4*1024*1024)

	h := &NodeHost{cmd: cmd, stdin: stdin, out: out}
	return h, nil
}

func (h *NodeHost) Close() error {
	if h == nil {
		return nil
	}
	_ = h.stdin.Close()
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
	if h.cmd != nil {
		_ = h.cmd.Wait()
	}
	return nil
}

func (h *NodeHost) roundTrip(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if h == nil {
		return nil, errors.New("nil NodeHost")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := h.nextID.Add(1)
	req := nodeRequest{ID: id, Method: method, Params: params}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, err := h.stdin.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	if !h.out.Scan() {
		if err := h.out.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("no response from plugin host")
	}
	var resp nodeResponse
	if err := json.Unmarshal(h.out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode host response: %w", err)
	}
	if resp.ID != id {
		return nil, fmt.Errorf("host response id mismatch: got %d want %d", resp.ID, id)
	}
	if !resp.OK {
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
		return nil, errors.New("plugin host error")
	}
	return resp.Result, nil
}

// Init loads plugins and returns the tool catalog.
func (h *NodeHost) Init(ctx context.Context, workspace string, sandboxed bool, pinchbotConfig map[string]any, plugins []DiscoveredPlugin) ([]CatalogTool, error) {
	list := make([]map[string]any, 0, len(plugins))
	for _, p := range plugins {
		entry := map[string]any{
			"id":   p.ID,
			"root": p.Root,
		}
		if len(p.PluginConfig) > 0 {
			entry["pluginConfig"] = p.PluginConfig
		}
		list = append(list, entry)
	}
	params := map[string]any{
		"workspace": workspace,
		"sandboxed": sandboxed,
		"plugins":   list,
	}
	if len(pinchbotConfig) > 0 {
		params["pinchbotConfig"] = pinchbotConfig
	}
	raw, err := h.roundTrip(ctx, "init", params)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Tools []CatalogTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("init result: %w", err)
	}
	return parsed.Tools, nil
}

// Execute runs a tool in the Node host.
func (h *NodeHost) Execute(ctx context.Context, pluginID, toolName string, args map[string]any) (string, error) {
	raw, err := h.roundTrip(ctx, "execute", map[string]any{
		"pluginId": pluginID,
		"tool":     toolName,
		"args":     args,
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("execute result: %w", err)
	}
	return parsed.Content, nil
}

// ContextOp runs context-engine or hook operations in the Node host (graph-memory, etc.).
func (h *NodeHost) ContextOp(ctx context.Context, params map[string]any) (json.RawMessage, error) {
	return h.roundTrip(ctx, "contextOp", params)
}
