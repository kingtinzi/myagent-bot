package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/sipeed/pinchbot/pkg/tools"
)

type blockedErrHost struct{}

func (blockedErrHost) Execute(context.Context, string, string, map[string]any) (string, error) {
	return "", errors.New("PINCHBOT_TOOL_BLOCKED: not allowed")
}

func TestBridgeTool_BlockedError_StripsPrefixForLLM(t *testing.T) {
	reg := tools.NewToolRegistry()
	reg.Register(NewBridgeTool(blockedErrHost{}, "p", "t1", "d", map[string]any{"type": "object", "properties": map[string]any{}}))
	res := reg.Execute(context.Background(), "t1", nil)
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
	if res.ForLLM != "not allowed" {
		t.Fatalf("ForLLM = %q, want stripped message without prefix", res.ForLLM)
	}
}

// ctxCapturingHost records ToolChannel / ToolChatID from the context passed to Execute
// (same path as ManagedPluginHost — cancellation + any future Go-side reads).
type ctxCapturingHost struct {
	lastChannel string
	lastChatID  string
}

func (h *ctxCapturingHost) Execute(ctx context.Context, pluginID, toolName string, args map[string]any) (string, error) {
	h.lastChannel = tools.ToolChannel(ctx)
	h.lastChatID = tools.ToolChatID(ctx)
	return "ok", nil
}

func TestBridgeTool_ExecuteWithContext_CarriesChannelChatID(t *testing.T) {
	h := &ctxCapturingHost{}
	reg := tools.NewToolRegistry()
	reg.Register(NewBridgeTool(h, "plugin-x", "bridge_ctx_tool", "test", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}))

	reg.ExecuteWithContext(context.Background(), "bridge_ctx_tool", map[string]any{}, "matrix", "room-1", nil)
	if h.lastChannel != "matrix" || h.lastChatID != "room-1" {
		t.Fatalf("expected channel matrix chat room-1, got channel=%q chatID=%q", h.lastChannel, h.lastChatID)
	}
}

// Ensures the same bridge path used by the agent loop (/ message path) matches HTTP invoke:
// registry injects channel/chat via WithToolContext before Tool.Execute.
func TestBridgeTool_InvokeStyleChannelMatchesAgentLoop(t *testing.T) {
	h := &ctxCapturingHost{}
	reg := tools.NewToolRegistry()
	reg.Register(NewBridgeTool(h, "plugin-x", "lobster_like", "test", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}))

	// Agent loop passes concrete channel + chat from the inbound message.
	reg.ExecuteWithContext(context.Background(), "lobster_like", nil, "telegram", "chat-42", nil)
	if h.lastChannel != "telegram" || h.lastChatID != "chat-42" {
		t.Fatalf("expected telegram/chat-42, got %q / %q", h.lastChannel, h.lastChatID)
	}

	// HTTP tools/invoke uses a synthetic channel and thread id (see toolsinvoke.Handler).
	reg.ExecuteWithContext(context.Background(), "lobster_like", nil, "http-invoke", "invoke", nil)
	if h.lastChannel != "http-invoke" || h.lastChatID != "invoke" {
		t.Fatalf("expected http-invoke/invoke, got %q / %q", h.lastChannel, h.lastChatID)
	}
}

func TestBridgeTool_Execute_WithoutWithContextLeavesEmpty(t *testing.T) {
	h := &ctxCapturingHost{}
	reg := tools.NewToolRegistry()
	reg.Register(NewBridgeTool(h, "plugin-x", "bridge_plain", "test", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}))

	reg.Execute(context.Background(), "bridge_plain", map[string]any{})
	if h.lastChannel != "" || h.lastChatID != "" {
		t.Fatalf("expected empty tool context, got channel=%q chatID=%q", h.lastChannel, h.lastChatID)
	}
}
