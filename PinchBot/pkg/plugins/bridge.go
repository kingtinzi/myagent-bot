package plugins

import (
	"context"

	"github.com/sipeed/pinchbot/pkg/tools"
)

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
	content, err := t.host.Execute(ctx, t.pluginID, t.name, args)
	if err != nil {
		return tools.ErrorResult(err.Error())
	}
	return tools.SilentResult(content)
}
