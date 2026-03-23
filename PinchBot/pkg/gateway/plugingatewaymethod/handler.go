// Package plugingatewaymethod serves POST /plugins/gateway-method: forwards to Node registerGatewayMethod handlers (IPC gatewayMethod).
// Authorization and rate limiting match pkg/gateway/toolsinvoke (shared with GET /plugins/status, POST /tools/invoke, and plugin HTTP routes).
package plugingatewaymethod

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/gateway/toolsinvoke"
	"github.com/sipeed/pinchbot/pkg/logger"
)

const maxBodyBytes = 2 * 1024 * 1024

// Handler forwards JSON-RPC-style gateway method calls to the default agent's Node plugin host.
type Handler struct {
	Cfg      *config.Config
	AgentReg func() *agent.AgentRegistry
}

type requestBody struct {
	PluginIDSnake string         `json:"plugin_id"`
	PluginIDCamel string         `json:"pluginId"`
	Method        string         `json:"method"`
	Params        map[string]any `json:"params"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":    false,
			"error": "POST required",
		})
		return
	}
	if h == nil || h.AgentReg == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	bearer := toolsinvoke.BearerFromRequest(r)
	if toolsinvoke.RateLimitExceeded(h.Cfg, r, bearer) {
		toolsinvoke.WriteRateLimitJSON(w)
		return
	}
	if !toolsinvoke.CheckGatewayInvokeAuth(h.Cfg, bearer) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	_ = r.Body.Close()

	var req requestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid JSON"})
		return
	}
	pluginID := strings.TrimSpace(req.PluginIDSnake)
	if pluginID == "" {
		pluginID = strings.TrimSpace(req.PluginIDCamel)
	}
	method := strings.TrimSpace(req.Method)
	if pluginID == "" || method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plugin_id and method are required"})
		return
	}

	reg := h.AgentReg()
	if reg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "no registry"})
		return
	}
	ag := reg.GetDefaultAgent()
	if ag == nil || ag.PluginHost == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node plugin host not enabled"})
		return
	}

	params := req.Params
	if params == nil {
		params = map[string]any{}
	}

	raw, err := ag.PluginHost.GatewayMethod(r.Context(), pluginID, method, params)
	if err != nil {
		logger.ErrorCF("gateway", "plugin gateway method", map[string]any{
			"error":     err.Error(),
			"plugin_id": pluginID,
			"method":    method,
		})
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}{OK: true, Result: raw})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
