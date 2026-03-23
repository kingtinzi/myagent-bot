package pluginhttp

import (
	"fmt"
	"strings"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/channels"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/logger"
)

// RegisterRoutes mounts plugin HTTP handlers on the shared Gateway mux (default agent's Node host only).
// Duplicate (method, path) from different plugins: first wins, others skipped with a warning.
func RegisterRoutes(m *channels.Manager, cfg *config.Config, reg *agent.AgentRegistry) {
	if m == nil || cfg == nil || reg == nil {
		return
	}
	ag := reg.GetDefaultAgent()
	if ag == nil || ag.PluginHost == nil {
		return
	}
	_, _, routes, _, _, _, _, _, _ := ag.PluginHost.InitSnapshot()
	if len(routes) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(routes))
	for _, rt := range routes {
		method := strings.ToUpper(strings.TrimSpace(rt.Method))
		pth := strings.TrimSpace(rt.Path)
		if method == "" || pth == "" || !strings.HasPrefix(pth, "/") {
			logger.WarnCF("gateway", "skipping invalid plugin http route", map[string]any{
				"plugin_id": rt.PluginID,
				"method":    rt.Method,
				"path":      rt.Path,
			})
			continue
		}
		key := method + " " + pth
		if _, ok := seen[key]; ok {
			logger.WarnCF("gateway", "skipping duplicate plugin http route", map[string]any{
				"plugin_id": rt.PluginID,
				"route":     key,
			})
			continue
		}
		seen[key] = struct{}{}
		rt.Method = method
		rt.Path = pth
		pat := fmt.Sprintf("%s %s", method, pth)
		m.RegisterHandler(pat, &Handler{Cfg: cfg, Host: ag.PluginHost, Route: rt})
		logger.InfoCF("gateway", "plugin HTTP route registered", map[string]any{
			"pattern":   pat,
			"plugin_id": rt.PluginID,
		})
	}
}
