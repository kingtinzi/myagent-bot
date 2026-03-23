package tools

import "context"

// BeforeToolCallHook runs after tool resolution and WithToolContext(channel, chatID), before Execute / ExecuteAsync.
// Use ToolAgentID(ctx) when the caller attached an agent id via WithAgentID (main loop, /tools/invoke, RunToolLoop).
// It implements the PinchBot **P0** “before_tool_call” extension point (pure Go; Node/OpenClaw parity is separate).
//
// Semantics:
//   - If block is non-nil, it is returned and the tool body is not executed.
//   - If block is nil and argsOut is non-nil, argsOut is passed to the tool.
//   - If block is nil and argsOut is nil, the hook leaves args unchanged (the registry passes a shallow clone of the original args).
//
// The hook receives a shallow clone of the incoming args map so callers' maps are not mutated unless the hook returns a new map.
type BeforeToolCallHook func(ctx context.Context, toolName string, args map[string]any) (argsOut map[string]any, block *ToolResult)

func cloneArgsShallow(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
