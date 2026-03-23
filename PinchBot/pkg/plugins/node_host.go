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

	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/tools"
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
	ID     int64           `json:"id"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// CatalogTool describes a tool registered by the Node host.
type CatalogTool struct {
	PluginID    string         `json:"pluginId"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// PluginHTTPRoute is metadata from Node registerHttpRoute. Handlers run in the plugin host;
// PinchBot Gateway does not yet dispatch HTTP to them (see runbook).
type PluginHTTPRoute struct {
	PluginID string `json:"pluginId"`
	Method   string `json:"method"`
	Path     string `json:"path"`
}

// PluginCLICommand is metadata from Node registerCommand (CLI execution is not wired in PinchBot).
type PluginCLICommand struct {
	PluginID    string `json:"pluginId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PluginGatewayMethod is metadata from Node registerGatewayMethod; handlers are invoked via IPC gatewayMethod and POST /plugins/gateway-method.
type PluginGatewayMethod struct {
	PluginID string `json:"pluginId"`
	Method   string `json:"method"`
}

// PluginRegisteredService is metadata from Node registerService (start/stop lifecycle not wired in PinchBot).
type PluginRegisteredService struct {
	PluginID  string `json:"pluginId"`
	ServiceID string `json:"serviceId"`
}

// PluginRegisterCli is metadata from one registerCli(registrar, opts?) call (CLI handler not wired in PinchBot).
type PluginRegisterCli struct {
	PluginID string   `json:"pluginId"`
	Commands []string `json:"commands"` // from opts.commands; may be empty
}

// PluginRegisteredProvider is metadata from Node registerProvider (inference/runtime not wired in PinchBot).
type PluginRegisteredProvider struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
}

// PluginProviderSnapshots groups registerProvider and registerSpeechProvider / registerMediaUnderstandingProvider /
// registerImageGenerationProvider / registerWebSearchProvider metadata (runtimes not wired in PinchBot).
type PluginProviderSnapshots struct {
	Text               []PluginRegisteredProvider
	Speech             []PluginRegisteredProvider
	MediaUnderstanding []PluginRegisteredProvider
	ImageGeneration    []PluginRegisteredProvider
	WebSearch          []PluginRegisteredProvider
}

// PluginRegisteredHook is metadata from Node registerHook (hooks are not executed in PinchBot).
type PluginRegisteredHook struct {
	PluginID string   `json:"pluginId"`
	Events   []string `json:"events"`
}

// PluginRegisteredChannel is metadata from Node registerChannel (channel runtime not wired in PinchBot).
type PluginRegisteredChannel struct {
	PluginID  string `json:"pluginId"`
	ChannelID string `json:"channelId"`
}

// PluginInteractiveHandlerRegistration is metadata from Node registerInteractiveHandler (handlers not wired in PinchBot).
type PluginInteractiveHandlerRegistration struct {
	PluginID  string `json:"pluginId"`
	Channel   string `json:"channel"`
	Namespace string `json:"namespace"`
}

// PluginConversationBindingListener is metadata from Node onConversationBindingResolved (not wired in PinchBot).
type PluginConversationBindingListener struct {
	PluginID string `json:"pluginId"`
}

// PluginInitExtras groups optional OpenClaw registration metadata beyond providers.
type PluginInitExtras struct {
	Hooks                        []PluginRegisteredHook
	Channels                     []PluginRegisteredChannel
	InteractiveHandlers          []PluginInteractiveHandlerRegistration
	ConversationBindingListeners []PluginConversationBindingListener
}

// PluginInitDiagnostic reports per-plugin load outcome from the Node host init pass.
type PluginInitDiagnostic struct {
	PluginID string `json:"pluginId"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

// LogPluginInitDiagnostics logs structured init results (failures at WARN, summary at INFO).
func LogPluginInitDiagnostics(diags []PluginInitDiagnostic) {
	if len(diags) == 0 {
		return
	}
	var okCount, failCount int
	for _, d := range diags {
		if d.OK {
			okCount++
			continue
		}
		failCount++
		fields := map[string]any{"plugin_id": d.PluginID}
		if strings.TrimSpace(d.Error) != "" {
			fields["error"] = d.Error
		}
		logger.WarnCF("plugins", "node-plugin-host: plugin init failed", fields)
	}
	logger.InfoCF("plugins", "node-plugin-host: init complete", map[string]any{
		"plugins_ok":     okCount,
		"plugins_failed": failCount,
	})
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
	// Plugin host logs unsupported OpenClaw APIs and per-plugin errors to stderr; keep visible for operators.
	cmd.Stderr = os.Stderr

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

// Init loads plugins and returns the tool catalog, diagnostics, declared HTTP routes, registerCommand metadata, gateway method declarations, registered plugin services, registerCli declarations, and register*Provider metadata (partial success: failed plugins are skipped).
func (h *NodeHost) Init(ctx context.Context, workspace string, sandboxed bool, pinchbotConfig map[string]any, plugins []DiscoveredPlugin) ([]CatalogTool, []PluginInitDiagnostic, []PluginHTTPRoute, []PluginCLICommand, []PluginGatewayMethod, []PluginRegisteredService, []PluginRegisterCli, PluginProviderSnapshots, PluginInitExtras, error) {
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
		return nil, nil, nil, nil, nil, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{}, err
	}
	var parsed struct {
		Tools                []CatalogTool                          `json:"tools"`
		Diagnostics          []PluginInitDiagnostic                 `json:"diagnostics"`
		HTTPRoutes           []PluginHTTPRoute                      `json:"httpRoutes"`
		CLICommands          []PluginCLICommand                     `json:"commands"`
		GatewayMethods       []PluginGatewayMethod                  `json:"gatewayMethods"`
		RegisteredServices   []PluginRegisteredService              `json:"registeredServices"`
		CliRegistrations     []PluginRegisterCli                    `json:"cliRegistrations"`
		RegisteredProviders  []PluginRegisteredProvider             `json:"registeredProviders"`
		SpeechProviders      []PluginRegisteredProvider             `json:"registeredSpeechProviders"`
		MediaProviders       []PluginRegisteredProvider             `json:"registeredMediaUnderstandingProviders"`
		ImageGenProviders    []PluginRegisteredProvider             `json:"registeredImageGenerationProviders"`
		WebSearchProviders   []PluginRegisteredProvider             `json:"registeredWebSearchProviders"`
		RegisteredHooks      []PluginRegisteredHook                 `json:"registeredHooks"`
		RegisteredChannels   []PluginRegisteredChannel              `json:"registeredChannels"`
		InteractiveHandlers  []PluginInteractiveHandlerRegistration `json:"registeredInteractiveHandlers"`
		ConvBindingListeners []PluginConversationBindingListener    `json:"conversationBindingResolvedListeners"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{}, fmt.Errorf("init result: %w", err)
	}
	prov := PluginProviderSnapshots{
		Text:               parsed.RegisteredProviders,
		Speech:             parsed.SpeechProviders,
		MediaUnderstanding: parsed.MediaProviders,
		ImageGeneration:    parsed.ImageGenProviders,
		WebSearch:          parsed.WebSearchProviders,
	}
	extras := PluginInitExtras{
		Hooks:                        parsed.RegisteredHooks,
		Channels:                     parsed.RegisteredChannels,
		InteractiveHandlers:          parsed.InteractiveHandlers,
		ConversationBindingListeners: parsed.ConvBindingListeners,
	}
	return parsed.Tools, parsed.Diagnostics, parsed.HTTPRoutes, parsed.CLICommands, parsed.GatewayMethods, parsed.RegisteredServices, parsed.CliRegistrations, prov, extras, nil
}

// Execute runs a tool in the Node host.
func (h *NodeHost) Execute(ctx context.Context, pluginID, toolName string, args map[string]any) (string, error) {
	params := map[string]any{
		"pluginId": pluginID,
		"tool":     toolName,
		"args":     args,
	}
	if c := tools.ToolChannel(ctx); c != "" {
		params["channel"] = c
	}
	if id := tools.ToolChatID(ctx); id != "" {
		params["chatId"] = id
	}
	if aid := tools.ToolAgentID(ctx); aid != "" {
		params["agentId"] = aid
	}
	raw, err := h.roundTrip(ctx, "execute", params)
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

// HTTPRouteResult is the decoded response from Node handleHttpRoute.
type HTTPRouteResult struct {
	Status  int               `json:"status"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
}

// HTTPRoute invokes a registerHttpRoute handler in the Node host (IPC method httpRoute).
func (h *NodeHost) HTTPRoute(ctx context.Context, pluginID, method, path string, query string, body []byte, headers map[string]any) (*HTTPRouteResult, error) {
	if headers == nil {
		headers = map[string]any{}
	}
	params := map[string]any{
		"pluginId": pluginID,
		"method":   method,
		"path":     path,
		"query":    query,
		"body":     string(body),
		"headers":  headers,
	}
	raw, err := h.roundTrip(ctx, "httpRoute", params)
	if err != nil {
		return nil, err
	}
	var parsed HTTPRouteResult
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("httpRoute result: %w", err)
	}
	if parsed.Headers == nil {
		parsed.Headers = map[string]string{}
	}
	return &parsed, nil
}

// GatewayMethod invokes a registerGatewayMethod handler in the Node host (IPC method gatewayMethod).
func (h *NodeHost) GatewayMethod(ctx context.Context, pluginID, method string, params map[string]any) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	p := map[string]any{
		"pluginId": pluginID,
		"method":   method,
		"params":   params,
	}
	return h.roundTrip(ctx, "gatewayMethod", p)
}

// ContextOp runs context-engine or hook operations in the Node host (graph-memory, etc.).
func (h *NodeHost) ContextOp(ctx context.Context, params map[string]any) (json.RawMessage, error) {
	return h.roundTrip(ctx, "contextOp", params)
}
