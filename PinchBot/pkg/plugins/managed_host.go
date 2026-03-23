package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ManagedPluginHost wraps NodeHost: restarts the Node process after IPC / process failures during Execute.
type ManagedPluginHost struct {
	opts ManagedHostOpts

	mu   sync.Mutex
	node *NodeHost

	closed atomic.Bool

	snapshotMu             sync.RWMutex
	lastCatalog            []CatalogTool
	lastDiagnostics        []PluginInitDiagnostic
	lastHTTPRoutes         []PluginHTTPRoute
	lastCLICommands        []PluginCLICommand
	lastGatewayMethods     []PluginGatewayMethod
	lastRegisteredServices []PluginRegisteredService
	lastCliRegistrations   []PluginRegisterCli
	lastProviderSnapshots  PluginProviderSnapshots
	lastInitExtras         PluginInitExtras
}

// ManagedHostOpts configures spawn, init, and execute-time recovery.
type ManagedHostOpts struct {
	NodeBinary string
	HostDir    string
	Workspace  string
	Sandboxed  bool
	Discovered []DiscoveredPlugin
	// PinchbotConfig is passed to plugins as api.config (e.g. graph-memory readProviderModel).
	PinchbotConfig map[string]any
	MaxRecoveries  int           // extra attempts after a failure (default 2)
	RestartDelay   time.Duration // backoff before respawn (default 500ms)
}

// BootstrapManagedHost starts Node, runs init once, and returns the catalog and per-plugin diagnostics.
func BootstrapManagedHost(ctx context.Context, opts ManagedHostOpts) (*ManagedPluginHost, []CatalogTool, []PluginInitDiagnostic, error) {
	// <0: no execute-time recovery; 0 (unset): default 2 extra attempts.
	if opts.MaxRecoveries < 0 {
		opts.MaxRecoveries = 0
	} else if opts.MaxRecoveries == 0 {
		opts.MaxRecoveries = 2
	}
	if opts.RestartDelay <= 0 {
		opts.RestartDelay = 500 * time.Millisecond
	}
	h, err := StartNodeHost(opts.NodeBinary, opts.HostDir, opts.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, prov, extras, err := h.Init(ctx, opts.Workspace, opts.Sandboxed, opts.PinchbotConfig, opts.Discovered)
	if err != nil {
		_ = h.Close()
		return nil, nil, nil, err
	}
	LogPluginInitDiagnostics(diags)
	mh := &ManagedPluginHost{opts: opts, node: h}
	mh.storeInitSnapshot(cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, prov, extras)
	return mh, cat, diags, nil
}

func (m *ManagedPluginHost) storeInitSnapshot(cat []CatalogTool, diags []PluginInitDiagnostic, httpRoutes []PluginHTTPRoute, cliCommands []PluginCLICommand, gatewayMethods []PluginGatewayMethod, registeredServices []PluginRegisteredService, cliRegistrations []PluginRegisterCli, providers PluginProviderSnapshots, extras PluginInitExtras) {
	if m == nil {
		return
	}
	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	m.lastCatalog = append([]CatalogTool(nil), cat...)
	m.lastDiagnostics = append([]PluginInitDiagnostic(nil), diags...)
	m.lastHTTPRoutes = append([]PluginHTTPRoute(nil), httpRoutes...)
	m.lastCLICommands = append([]PluginCLICommand(nil), cliCommands...)
	m.lastGatewayMethods = append([]PluginGatewayMethod(nil), gatewayMethods...)
	m.lastRegisteredServices = append([]PluginRegisteredService(nil), registeredServices...)
	m.lastCliRegistrations = append([]PluginRegisterCli(nil), cliRegistrations...)
	m.lastProviderSnapshots = PluginProviderSnapshots{
		Text:               append([]PluginRegisteredProvider(nil), providers.Text...),
		Speech:             append([]PluginRegisteredProvider(nil), providers.Speech...),
		MediaUnderstanding: append([]PluginRegisteredProvider(nil), providers.MediaUnderstanding...),
		ImageGeneration:    append([]PluginRegisteredProvider(nil), providers.ImageGeneration...),
		WebSearch:          append([]PluginRegisteredProvider(nil), providers.WebSearch...),
	}
	m.lastInitExtras = PluginInitExtras{
		Hooks:                        append([]PluginRegisteredHook(nil), extras.Hooks...),
		Channels:                     append([]PluginRegisteredChannel(nil), extras.Channels...),
		InteractiveHandlers:          append([]PluginInteractiveHandlerRegistration(nil), extras.InteractiveHandlers...),
		ConversationBindingListeners: append([]PluginConversationBindingListener(nil), extras.ConversationBindingListeners...),
	}
}

// InitSnapshot returns a copy of the last successful Node init snapshot (after bootstrap or respawn).
func (m *ManagedPluginHost) InitSnapshot() ([]CatalogTool, []PluginInitDiagnostic, []PluginHTTPRoute, []PluginCLICommand, []PluginGatewayMethod, []PluginRegisteredService, []PluginRegisterCli, PluginProviderSnapshots, PluginInitExtras) {
	if m == nil {
		return nil, nil, nil, nil, nil, nil, nil, PluginProviderSnapshots{}, PluginInitExtras{}
	}
	m.snapshotMu.RLock()
	defer m.snapshotMu.RUnlock()
	ps := m.lastProviderSnapshots
	ex := m.lastInitExtras
	return append([]CatalogTool(nil), m.lastCatalog...),
		append([]PluginInitDiagnostic(nil), m.lastDiagnostics...),
		append([]PluginHTTPRoute(nil), m.lastHTTPRoutes...),
		append([]PluginCLICommand(nil), m.lastCLICommands...),
		append([]PluginGatewayMethod(nil), m.lastGatewayMethods...),
		append([]PluginRegisteredService(nil), m.lastRegisteredServices...),
		append([]PluginRegisterCli(nil), m.lastCliRegistrations...),
		PluginProviderSnapshots{
			Text:               append([]PluginRegisteredProvider(nil), ps.Text...),
			Speech:             append([]PluginRegisteredProvider(nil), ps.Speech...),
			MediaUnderstanding: append([]PluginRegisteredProvider(nil), ps.MediaUnderstanding...),
			ImageGeneration:    append([]PluginRegisteredProvider(nil), ps.ImageGeneration...),
			WebSearch:          append([]PluginRegisteredProvider(nil), ps.WebSearch...),
		},
		PluginInitExtras{
			Hooks:                        append([]PluginRegisteredHook(nil), ex.Hooks...),
			Channels:                     append([]PluginRegisteredChannel(nil), ex.Channels...),
			InteractiveHandlers:          append([]PluginInteractiveHandlerRegistration(nil), ex.InteractiveHandlers...),
			ConversationBindingListeners: append([]PluginConversationBindingListener(nil), ex.ConversationBindingListeners...),
		}
}

// Close stops the Node process.
func (m *ManagedPluginHost) Close() error {
	if m == nil {
		return nil
	}
	m.closed.Store(true)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.node != nil {
		err := m.node.Close()
		m.node = nil
		return err
	}
	return nil
}

// Execute runs a tool, restarting the host on recoverable IPC / process errors.
func (m *ManagedPluginHost) Execute(ctx context.Context, pluginID, toolName string, args map[string]any) (string, error) {
	if m == nil {
		return "", errors.New("nil ManagedPluginHost")
	}
	if m.closed.Load() {
		return "", errors.New("plugin host closed")
	}

	maxAttempts := 1 + m.opts.MaxRecoveries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(m.opts.RestartDelay):
			}
		}

		m.mu.Lock()
		if m.node == nil {
			h, err := StartNodeHost(m.opts.NodeBinary, m.opts.HostDir, m.opts.Workspace)
			if err != nil {
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return "", err
				}
				continue
			}
			cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras, err := h.Init(ctx, m.opts.Workspace, m.opts.Sandboxed, m.opts.PinchbotConfig, m.opts.Discovered)
			if err != nil {
				_ = h.Close()
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return "", err
				}
				continue
			}
			LogPluginInitDiagnostics(diags)
			m.storeInitSnapshot(cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras)
			m.node = h
		}
		n := m.node
		m.mu.Unlock()

		out, err := n.Execute(ctx, pluginID, toolName, args)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		if !isRecoverableHostError(err) {
			return "", err
		}

		m.mu.Lock()
		if m.node == n {
			_ = m.node.Close()
			m.node = nil
		}
		m.mu.Unlock()
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("plugin host: execute failed")
}

// HTTPRoute forwards to the Node host with the same respawn semantics as Execute.
func (m *ManagedPluginHost) HTTPRoute(ctx context.Context, pluginID, method, path string, query string, body []byte, headers map[string]any) (*HTTPRouteResult, error) {
	if m == nil {
		return nil, errors.New("nil ManagedPluginHost")
	}
	if m.closed.Load() {
		return nil, errors.New("plugin host closed")
	}

	maxAttempts := 1 + m.opts.MaxRecoveries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(m.opts.RestartDelay):
			}
		}

		m.mu.Lock()
		if m.node == nil {
			h, err := StartNodeHost(m.opts.NodeBinary, m.opts.HostDir, m.opts.Workspace)
			if err != nil {
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras, err := h.Init(ctx, m.opts.Workspace, m.opts.Sandboxed, m.opts.PinchbotConfig, m.opts.Discovered)
			if err != nil {
				_ = h.Close()
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			LogPluginInitDiagnostics(diags)
			m.storeInitSnapshot(cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras)
			m.node = h
		}
		n := m.node
		m.mu.Unlock()

		out, err := n.HTTPRoute(ctx, pluginID, method, path, query, body, headers)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if !isRecoverableHostError(err) {
			return nil, err
		}

		m.mu.Lock()
		if m.node == n {
			_ = m.node.Close()
			m.node = nil
		}
		m.mu.Unlock()
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("plugin host: httpRoute failed")
}

// GatewayMethod forwards to the Node host with the same respawn semantics as HTTPRoute.
func (m *ManagedPluginHost) GatewayMethod(ctx context.Context, pluginID, method string, params map[string]any) (json.RawMessage, error) {
	if m == nil {
		return nil, errors.New("nil ManagedPluginHost")
	}
	if m.closed.Load() {
		return nil, errors.New("plugin host closed")
	}

	maxAttempts := 1 + m.opts.MaxRecoveries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(m.opts.RestartDelay):
			}
		}

		m.mu.Lock()
		if m.node == nil {
			h, err := StartNodeHost(m.opts.NodeBinary, m.opts.HostDir, m.opts.Workspace)
			if err != nil {
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras, err := h.Init(ctx, m.opts.Workspace, m.opts.Sandboxed, m.opts.PinchbotConfig, m.opts.Discovered)
			if err != nil {
				_ = h.Close()
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			LogPluginInitDiagnostics(diags)
			m.storeInitSnapshot(cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras)
			m.node = h
		}
		n := m.node
		m.mu.Unlock()

		out, err := n.GatewayMethod(ctx, pluginID, method, params)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if !isRecoverableHostError(err) {
			return nil, err
		}

		m.mu.Lock()
		if m.node == n {
			_ = m.node.Close()
			m.node = nil
		}
		m.mu.Unlock()
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("plugin host: gatewayMethod failed")
}

// ContextOp forwards to the Node host with the same respawn semantics as Execute.
func (m *ManagedPluginHost) ContextOp(ctx context.Context, params map[string]any) (json.RawMessage, error) {
	if m == nil {
		return nil, errors.New("nil ManagedPluginHost")
	}
	if m.closed.Load() {
		return nil, errors.New("plugin host closed")
	}

	maxAttempts := 1 + m.opts.MaxRecoveries
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(m.opts.RestartDelay):
			}
		}

		m.mu.Lock()
		if m.node == nil {
			h, err := StartNodeHost(m.opts.NodeBinary, m.opts.HostDir, m.opts.Workspace)
			if err != nil {
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras, err := h.Init(ctx, m.opts.Workspace, m.opts.Sandboxed, m.opts.PinchbotConfig, m.opts.Discovered)
			if err != nil {
				_ = h.Close()
				m.mu.Unlock()
				lastErr = err
				if !isRecoverableHostError(err) {
					return nil, err
				}
				continue
			}
			LogPluginInitDiagnostics(diags)
			m.storeInitSnapshot(cat, diags, httpRoutes, cliCommands, gwMethods, regSvcs, cliRegs, providers, extras)
			m.node = h
		}
		n := m.node
		m.mu.Unlock()

		out, err := n.ContextOp(ctx, params)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if !isRecoverableHostError(err) {
			return nil, err
		}

		m.mu.Lock()
		if m.node == n {
			_ = m.node.Close()
			m.node = nil
		}
		m.mu.Unlock()
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("plugin host: contextOp failed")
}

func isRecoverableHostError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// Tool / plugin logic errors — do not respawn.
	if strings.Contains(s, "unknown tool") {
		return false
	}
	if strings.Contains(s, "no gateway method handler") {
		return false
	}
	if strings.Contains(s, "no http route handler") {
		return false
	}
	if strings.Contains(s, "invalid json") && strings.Contains(s, "host") {
		return false
	}
	return strings.Contains(s, "no response") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "signal: killed") ||
		strings.Contains(s, "already closed") ||
		strings.Contains(s, "unexpected eof") ||
		strings.Contains(s, "write |") ||
		strings.Contains(s, "read |") ||
		strings.Contains(s, "connection reset")
}
