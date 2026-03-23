// Package pluginsstatus exposes GET /plugins/status for operator visibility (loaded extensions + init diagnostics).
// Authorization and rate limiting match pkg/gateway/toolsinvoke (shared with POST /tools/invoke, POST /plugins/gateway-method, and plugin HTTP routes).
package pluginsstatus

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/gateway/toolsinvoke"
	"github.com/sipeed/pinchbot/pkg/plugins"
)

// Handler serves GET /plugins/status on the Gateway mux.
type Handler struct {
	Cfg      *config.Config
	AgentReg func() *agent.AgentRegistry
}

type agentPluginsPayload struct {
	AgentID string                     `json:"agent_id"`
	Plugins []plugins.PluginInitStatus `json:"plugins"`
}

type response struct {
	OK                                bool                  `json:"ok"`
	NodeHost                          bool                  `json:"node_host"`
	ExtensionsDir                     string                `json:"extensions_dir,omitempty"`
	PluginsEnabled                    []string              `json:"plugins_enabled,omitempty"`
	GatewayRateLimitRequestsPerMinute int                   `json:"gateway_rate_limit_requests_per_minute,omitempty"`
	Agents                            []agentPluginsPayload `json:"agents"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.AgentReg == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg := h.Cfg
	bearer := toolsinvoke.BearerFromRequest(r)
	if toolsinvoke.RateLimitExceeded(cfg, r, bearer) {
		toolsinvoke.WriteRateLimitJSON(w)
		return
	}
	if !toolsinvoke.CheckGatewayInvokeAuth(cfg, bearer) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"ok":    false,
			"error": "unauthorized",
		})
		return
	}
	reg := h.AgentReg()
	if reg == nil {
		writeJSON(w, http.StatusOK, response{OK: true, Agents: nil})
		return
	}

	var extDir string
	if cfg != nil {
		if ag := reg.GetDefaultAgent(); ag != nil && ag.Workspace != "" {
			if p, err := plugins.ResolveExtensionsDir(ag.Workspace, cfg.Plugins.ExtensionsDir); err == nil {
				extDir = p
			}
		}
	}

	enabled := []string(nil)
	nodeHost := false
	if cfg != nil {
		enabled = cfg.Plugins.EffectiveEnabledPluginIDs()
		nodeHost = cfg.Plugins.NodeHost
	}

	ids := reg.ListAgentIDs()
	sort.Strings(ids)
	agents := make([]agentPluginsPayload, 0, len(ids))
	for _, id := range ids {
		ag, ok := reg.GetAgent(id)
		if !ok || ag == nil {
			continue
		}
		rows := ag.NodePluginStatus()
		agents = append(agents, agentPluginsPayload{AgentID: id, Plugins: rows})
	}

	resp := response{
		OK:             true,
		NodeHost:       nodeHost,
		ExtensionsDir:  extDir,
		PluginsEnabled: enabled,
		Agents:         agents,
	}
	if rpm := toolsinvoke.GatewayRateLimitFromConfig(cfg); rpm > 0 {
		resp.GatewayRateLimitRequestsPerMinute = rpm
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
