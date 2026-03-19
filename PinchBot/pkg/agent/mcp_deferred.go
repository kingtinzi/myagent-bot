package agent

import "github.com/sipeed/pinchbot/pkg/config"

// serverIsDeferred reports whether an MCP server's tools should be registered
// as hidden (deferred).
func serverIsDeferred(serverCfg config.MCPServerConfig) bool {
	return serverCfg.Deferred != nil && *serverCfg.Deferred
}
