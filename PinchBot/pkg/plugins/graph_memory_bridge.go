package plugins

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/providers"
)

const DefaultGraphMemoryEngineID = "graph-memory"

// GraphMemoryRuntimeActive is true when sidecar config, node host, plugin id, and host process are ready.
func GraphMemoryRuntimeActive(cfg *config.Config, host *ManagedPluginHost) bool {
	if cfg == nil || host == nil {
		return false
	}
	if cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		return false
	}
	if !cfg.Plugins.NodeHost || !cfg.Plugins.IsPluginEnabled(DefaultGraphMemoryEngineID) {
		return false
	}
	return true
}

// ConversationToWire maps PinchBot messages to JSON values for the graph-memory assemble pipeline.
func ConversationToWire(msgs []providers.Message) ([]any, error) {
	out := make([]any, 0, len(msgs))
	for _, msg := range msgs {
		b, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		var obj map[string]any
		if err := json.Unmarshal(b, &obj); err != nil {
			return nil, err
		}
		if msg.ToolCallID != "" {
			obj["toolCallId"] = msg.ToolCallID
		}
		out = append(out, obj)
	}
	return out, nil
}

// MergeSystemPromptAddition appends graph-memory systemPromptAddition to the first (system) message.
func MergeSystemPromptAddition(messages []providers.Message, addition string) []providers.Message {
	addition = strings.TrimSpace(addition)
	if addition == "" || len(messages) == 0 {
		return messages
	}
	sys := messages[0]
	sys.Content = strings.TrimSpace(sys.Content) + "\n\n---\n\n" + addition
	if sys.SystemParts == nil {
		sys.SystemParts = []providers.ContentBlock{}
	}
	sys.SystemParts = append(sys.SystemParts, providers.ContentBlock{Type: "text", Text: addition})
	messages[0] = sys
	return messages
}

// ParseAssembleResult decodes the Node context engine assemble() return value.
func ParseAssembleResult(raw json.RawMessage) (systemAddition string, tail []providers.Message, _ error) {
	var aux struct {
		Messages             json.RawMessage `json:"messages"`
		SystemPromptAddition string          `json:"systemPromptAddition"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return "", nil, err
	}
	systemAddition = strings.TrimSpace(aux.SystemPromptAddition)
	if len(aux.Messages) == 0 || string(aux.Messages) == "null" {
		return systemAddition, nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(aux.Messages, &arr); err != nil {
		// single message?
		var one providers.Message
		if err2 := json.Unmarshal(aux.Messages, &one); err2 == nil && strings.TrimSpace(one.Role) != "" {
			return systemAddition, []providers.Message{one}, nil
		}
		return systemAddition, nil, err
	}
	for _, r := range arr {
		msg, err := wireJSONToMessage(r)
		if err != nil {
			return systemAddition, nil, err
		}
		tail = append(tail, msg)
	}
	return systemAddition, tail, nil
}

func wireJSONToMessage(raw json.RawMessage) (providers.Message, error) {
	var msg providers.Message
	if err := json.Unmarshal(raw, &msg); err == nil && strings.TrimSpace(msg.Role) != "" {
		return normalizeWireMessage(&msg), nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return providers.Message{}, err
	}
	role, _ := m["role"].(string)
	out := providers.Message{Role: role}
	if id, ok := m["toolCallId"].(string); ok && id != "" {
		out.ToolCallID = id
	}
	if id, ok := m["tool_call_id"].(string); ok && id != "" && out.ToolCallID == "" {
		out.ToolCallID = id
	}
	switch c := m["content"].(type) {
	case string:
		out.Content = c
	case []any, map[string]any:
		b, err := json.Marshal(c)
		if err != nil {
			return out, err
		}
		out.Content = string(b)
	}
	if tc, ok := m["tool_calls"].([]any); ok && len(tc) > 0 {
		b, err := json.Marshal(tc)
		if err != nil {
			return out, err
		}
		var calls []providers.ToolCall
		if err := json.Unmarshal(b, &calls); err == nil {
			out.ToolCalls = calls
		}
	}
	return normalizeWireMessage(&out), nil
}

func normalizeWireMessage(m *providers.Message) providers.Message {
	if m == nil {
		return providers.Message{}
	}
	return *m
}

// EmitGraphMemoryBeforeAgentStart runs before_agent_start hooks in the Node host.
func EmitGraphMemoryBeforeAgentStart(ctx context.Context, host *ManagedPluginHost, prompt, sessionKey string) {
	if host == nil || ctx.Err() != nil {
		return
	}
	_, err := host.ContextOp(ctx, map[string]any{
		"op":    "emit",
		"event": "before_agent_start",
		"eventPayload": map[string]any{
			"prompt": strings.TrimSpace(prompt),
		},
		"ctx": map[string]any{
			"sessionKey": sessionKey,
			"sessionId":  sessionKey,
		},
	})
	if err != nil {
		logger.WarnCF("plugins", "graph-memory before_agent_start: "+err.Error(), map[string]any{
			"session_key": sessionKey,
		})
	}
}

// AssembleGraphMemory calls the context engine assemble hook.
func AssembleGraphMemory(ctx context.Context, host *ManagedPluginHost, engineID, sessionKey string, wire []any, tokenBudget int) (systemAddition string, tail []providers.Message, err error) {
	if host == nil {
		return "", nil, nil
	}
	if engineID == "" {
		engineID = DefaultGraphMemoryEngineID
	}
	raw, err := host.ContextOp(ctx, map[string]any{
		"op":          "assemble",
		"engineId":    engineID,
		"sessionId":   sessionKey,
		"messages":    wire,
		"tokenBudget": tokenBudget,
	})
	if err != nil {
		return "", nil, err
	}
	return ParseAssembleResult(raw)
}

// AfterTurnGraphMemory calls the context engine afterTurn hook (ingest + async extract in plugin).
func AfterTurnGraphMemory(ctx context.Context, host *ManagedPluginHost, engineID, sessionKey string, wire []any, prePromptCount int, isHeartbeat bool) {
	if host == nil || ctx.Err() != nil {
		return
	}
	if engineID == "" {
		engineID = DefaultGraphMemoryEngineID
	}
	_, err := host.ContextOp(ctx, map[string]any{
		"op":                   "afterTurn",
		"engineId":             engineID,
		"sessionId":            sessionKey,
		"messages":             wire,
		"prePromptMessageCount": prePromptCount,
		"isHeartbeat":          isHeartbeat,
	})
	if err != nil {
		logger.WarnCF("plugins", "graph-memory afterTurn: "+err.Error(), map[string]any{
			"session_key": sessionKey,
		})
	}
}
