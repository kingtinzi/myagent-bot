package plugins

import (
	"context"
	"strings"

	"github.com/sipeed/pinchbot/pkg/tools"
)

const pinchbotToolBlockedPrefix = "PINCHBOT_TOOL_BLOCKED:"

func mapBridgeExecuteError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.HasPrefix(msg, pinchbotToolBlockedPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(msg, pinchbotToolBlockedPrefix))
	}
	return msg
}

// PluginBridgeExecutor runs a plugin tool (NodeHost or ManagedPluginHost).
type PluginBridgeExecutor interface {
	Execute(ctx context.Context, pluginID, toolName string, args map[string]any) (string, error)
}

type bridgeTool struct {
	host        PluginBridgeExecutor
	pluginID    string
	name        string
	description string
	params      map[string]any
}

// NewBridgeTool wraps a tool executed by the Node plugin host.
func NewBridgeTool(host PluginBridgeExecutor, pluginID, name, description string, params map[string]any) tools.Tool {
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return &bridgeTool{
		host:        host,
		pluginID:    pluginID,
		name:        name,
		description: description,
		params:      params,
	}
}

func (t *bridgeTool) Name() string {
	return t.name
}

func (t *bridgeTool) Description() string {
	return t.description
}

func (t *bridgeTool) Parameters() map[string]any {
	return t.params
}

func (t *bridgeTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	// ctx is produced by tools.ToolRegistry.ExecuteWithContext (agent loop, /tools/invoke, cron, etc.),
	// which wraps WithToolContext — PluginBridgeExecutor sees the same channel/chatID as native tools.
	content, err := t.host.Execute(ctx, t.pluginID, t.name, args)
	if err != nil {
		return tools.ErrorResult(mapBridgeExecuteError(err))
	}
	return tools.SilentResult(content)
}
