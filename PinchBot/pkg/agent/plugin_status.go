package agent

import "github.com/sipeed/pinchbot/pkg/plugins"

// NodePluginStatus returns Node host snapshot for this agent (empty if no host).
func (a *AgentInstance) NodePluginStatus() []plugins.PluginInitStatus {
	if a == nil || a.PluginHost == nil {
		return nil
	}
	cat, diags, httpRoutes, cliCommands, gatewayMethods, registeredServices, registerCli, providerSnap, extras := a.PluginHost.InitSnapshot()
	return plugins.BuildPluginInitStatus(cat, diags, httpRoutes, cliCommands, gatewayMethods, registeredServices, registerCli, providerSnap, extras)
}
