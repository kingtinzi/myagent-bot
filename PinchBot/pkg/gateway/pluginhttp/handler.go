// Package pluginhttp exposes Gateway HTTP routes declared by Node extensions (registerHttpRoute).
// Authorization and rate limiting match pkg/gateway/toolsinvoke (shared with POST /tools/invoke, GET /plugins/status, POST /plugins/gateway-method).
package pluginhttp

import (
	"io"
	"net/http"
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/gateway/toolsinvoke"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/plugins"
)

// Handler forwards a single registered plugin HTTP route to the Node host (same auth as /tools/invoke).
type Handler struct {
	Cfg   *config.Config
	Host  *plugins.ManagedPluginHost
	Route plugins.PluginHTTPRoute
}

const maxBodyBytes = 2 * 1024 * 1024

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Host == nil || h.Cfg == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimSpace(h.Route.Path)
	if path == "" {
		http.Error(w, "bad route", http.StatusInternalServerError)
		return
	}

	bearer := toolsinvoke.BearerFromRequest(r)
	if toolsinvoke.RateLimitExceeded(h.Cfg, r, bearer) {
		toolsinvoke.WriteRateLimitJSON(w)
		return
	}
	if !toolsinvoke.CheckGatewayInvokeAuth(h.Cfg, bearer) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	query := r.URL.RawQuery
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
	}

	hdrs := headersToAny(r.Header)
	res, err := h.Host.HTTPRoute(ctx, h.Route.PluginID, r.Method, path, query, body, hdrs)
	if err != nil {
		logger.ErrorCF("gateway", "plugin http route", map[string]any{
			"error":  err.Error(),
			"plugin": h.Route.PluginID,
			"path":   path,
		})
		http.Error(w, "plugin host error", http.StatusBadGateway)
		return
	}
	if res == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	st := res.Status
	if st < 100 || st > 599 {
		st = http.StatusOK
	}
	for k, v := range res.Headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		w.Header().Set(k, v)
	}
	w.WriteHeader(st)
	if res.Body != "" {
		_, _ = io.WriteString(w, res.Body)
	}
}

func headersToAny(h http.Header) map[string]any {
	if h == nil {
		return nil
	}
	out := make(map[string]any)
	n := 0
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		if n >= 64 {
			break
		}
		out[k] = v[0]
		n++
	}
	return out
}
