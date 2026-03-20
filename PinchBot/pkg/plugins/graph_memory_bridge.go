package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/graphmemory"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/providers"
)

const DefaultGraphMemoryEngineID = "graph-memory"

// graphMemoryNativeImplemented marks whether the Go-native graph-memory runtime
// has fully replaced the Node bridge path.
const graphMemoryNativeImplemented = true

// GraphMemoryUseNodeBridge returns true when runtime should still call Node host.
func GraphMemoryUseNodeBridge(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if !cfg.Plugins.GraphMemoryGoNative {
		return true
	}
	return !graphMemoryNativeImplemented
}

// GraphMemoryRuntimeActive is true when sidecar + plugin enablement are ready
// and either native runtime is available or Node bridge host is ready.
func GraphMemoryRuntimeActive(cfg *config.Config, host *ManagedPluginHost) bool {
	if cfg == nil {
		return false
	}
	if cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		return false
	}
	if !cfg.Plugins.IsPluginEnabled(DefaultGraphMemoryEngineID) {
		return false
	}
	if GraphMemoryUseNodeBridge(cfg) {
		if !cfg.Plugins.NodeHost || host == nil {
			return false
		}
		return true
	}
	return graphMemoryNativeImplemented
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
func EmitGraphMemoryBeforeAgentStart(ctx context.Context, cfg *config.Config, host *ManagedPluginHost, prompt, sessionKey string) {
	if !GraphMemoryUseNodeBridge(cfg) || host == nil || ctx.Err() != nil {
		return
	}
	emitGraphMemoryBeforeAgentStartNode(ctx, host, prompt, sessionKey)
}

func emitGraphMemoryBeforeAgentStartNode(ctx context.Context, host *ManagedPluginHost, prompt, sessionKey string) {
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
func AssembleGraphMemory(
	ctx context.Context,
	cfg *config.Config,
	host *ManagedPluginHost,
	engineID, sessionKey string,
	messages []providers.Message,
	tokenBudget int,
) (systemAddition string, tail []providers.Message, err error) {
	if GraphMemoryUseNodeBridge(cfg) {
		wire, convErr := ConversationToWire(messages)
		if convErr != nil {
			return "", nil, convErr
		}
		return assembleGraphMemoryNode(ctx, host, engineID, sessionKey, wire, tokenBudget)
	}
	return assembleGraphMemoryNative(cfg, sessionKey, messages)
}

func assembleGraphMemoryNode(ctx context.Context, host *ManagedPluginHost, engineID, sessionKey string, wire []any, tokenBudget int) (systemAddition string, tail []providers.Message, err error) {
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
func AfterTurnGraphMemory(
	ctx context.Context,
	cfg *config.Config,
	host *ManagedPluginHost,
	engineID, sessionKey string,
	messages []providers.Message,
	prePromptCount int,
	isHeartbeat bool,
) {
	if ctx.Err() != nil {
		return
	}
	if GraphMemoryUseNodeBridge(cfg) {
		wire, convErr := ConversationToWire(messages)
		if convErr != nil {
			logger.WarnCF("plugins", "graph-memory afterTurn wire failed", map[string]any{"error": convErr.Error()})
			return
		}
		afterTurnGraphMemoryNode(ctx, host, engineID, sessionKey, wire, prePromptCount, isHeartbeat)
		return
	}
	afterTurnGraphMemoryNative(cfg, sessionKey, messages, prePromptCount, isHeartbeat)
}

func afterTurnGraphMemoryNode(ctx context.Context, host *ManagedPluginHost, engineID, sessionKey string, wire []any, prePromptCount int, isHeartbeat bool) {
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

func assembleGraphMemoryNative(cfg *config.Config, sessionKey string, messages []providers.Message) (string, []providers.Message, error) {
	if !graphmemory.Enabled(cfg) {
		return "", nil, nil
	}
	if len(messages) == 0 {
		return "", nil, nil
	}
	query := strings.TrimSpace(messages[len(messages)-1].Content)
	if query == "" {
		return "", nil, nil
	}
	dbPath := graphmemory.ResolveDBPath(cfg)
	db, err := graphmemory.OpenDB(dbPath)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()
	nodes, err := graphmemory.SearchNodes(db, query, graphMemoryRecallMaxNodes(cfg))
	if err != nil {
		return "", nil, err
	}
	addition := graphmemory.BuildSystemPromptAddition(query, nodes)
	return addition, nil, nil
}

func afterTurnGraphMemoryNative(cfg *config.Config, sessionKey string, messages []providers.Message, prePromptCount int, isHeartbeat bool) {
	if !graphmemory.Enabled(cfg) || isHeartbeat || len(messages) == 0 {
		return
	}
	if prePromptCount < 0 {
		prePromptCount = 0
	}
	if prePromptCount >= len(messages) {
		return
	}
	dbPath := graphmemory.ResolveDBPath(cfg)
	db, err := graphmemory.OpenDB(dbPath)
	if err != nil {
		logger.WarnCF("plugins", "graph-memory native open db failed", map[string]any{"error": err.Error()})
		return
	}
	defer db.Close()
	if err := graphmemory.SaveMessages(db, sessionKey, prePromptCount, messages[prePromptCount:]); err != nil {
		logger.WarnCF("plugins", "graph-memory native save messages failed", map[string]any{"error": err.Error()})
	}
	if graphMemoryAutoExtractEnabled(cfg) {
		if err := graphmemory.AutoExtractFromMessages(db, sessionKey, messages[prePromptCount:]); err != nil {
			logger.WarnCF("plugins", "graph-memory native auto extract failed", map[string]any{"error": err.Error()})
		}
	}
	if graphMemoryLLMExtractEnabled(cfg) {
		if err := maybeLLMExtractNative(context.Background(), db, cfg, sessionKey, messages[prePromptCount:]); err != nil {
			logger.WarnCF("plugins", "graph-memory native llm extract failed", map[string]any{"error": err.Error()})
		}
	}
	if graphMemoryAutoMaintainEnabled(cfg) {
		if err := maybeMaintainNative(db, cfg, sessionKey); err != nil {
			logger.WarnCF("plugins", "graph-memory native maintain failed", map[string]any{"error": err.Error()})
		}
	}
}

func graphMemoryAutoExtractEnabled(cfg *config.Config) bool {
	// default true
	return graphMemorySidecarBool(cfg, "nativeAutoExtract", true)
}

func graphMemoryAutoMaintainEnabled(cfg *config.Config) bool {
	// default true
	return graphMemorySidecarBool(cfg, "nativeAutoMaintain", true)
}

func graphMemoryLLMExtractEnabled(cfg *config.Config) bool {
	// default false
	return graphMemorySidecarBool(cfg, "nativeLLMExtract", false)
}

func graphMemoryMaintainEveryTurns(cfg *config.Config) int {
	// default 20
	return graphMemorySidecarInt(cfg, "nativeMaintainEveryTurns", 20)
}

func graphMemoryLLMExtractMaxNodes(cfg *config.Config) int {
	// default 3
	return graphMemorySidecarInt(cfg, "nativeLLMExtractMaxNodes", 3)
}

func graphMemoryRecallMaxNodes(cfg *config.Config) int {
	// default 8
	return graphMemorySidecarInt(cfg, "recallMaxNodes", 8)
}

// GraphMemoryRecentWindow returns near-field raw-history window size when graph-memory is enabled.
// default 20.
func GraphMemoryRecentWindow(cfg *config.Config) int {
	v := graphMemorySidecarInt(cfg, "recentWindow", 20)
	if v < 4 {
		return 4
	}
	if v > 100 {
		return 100
	}
	return v
}

func graphMemorySidecarBool(cfg *config.Config, key string, def bool) bool {
	if cfg == nil || cfg.GraphMemory == nil || cfg.GraphMemory.Raw == nil {
		return def
	}
	v, ok := cfg.GraphMemory.Raw[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "false" || s == "0" || s == "no" {
			return false
		}
	case float64:
		return t != 0
	}
	return def
}

func graphMemorySidecarInt(cfg *config.Config, key string, def int) int {
	if cfg == nil || cfg.GraphMemory == nil || cfg.GraphMemory.Raw == nil {
		return def
	}
	v, ok := cfg.GraphMemory.Raw[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		if int(t) > 0 {
			return int(t)
		}
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func maybeMaintainNative(db *sql.DB, cfg *config.Config, sessionKey string) error {
	every := graphMemoryMaintainEveryTurns(cfg)
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_messages WHERE session_id=?`, sessionKey).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 || cnt%every != 0 {
		return nil
	}
	updated, err := graphmemory.RecomputePageRankHeuristic(db)
	if err != nil {
		return err
	}
	logger.InfoCF("plugins", "graph-memory native auto maintain done", map[string]any{
		"session_key": sessionKey,
		"messages":    cnt,
		"updated":     updated,
	})
	return nil
}

type llmExtractNode struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	RelatedTo   string `json:"relatedTo"`
}

func maybeLLMExtractNative(ctx context.Context, db *sql.DB, cfg *config.Config, sessionKey string, messages []providers.Message) error {
	llmCfg := graphMemorySidecarObject(cfg, "llm")
	if llmCfg == nil {
		return nil
	}
	model := strings.TrimSpace(readString(llmCfg, "model"))
	if model == "" {
		return nil
	}
	apiKey := strings.TrimSpace(readString(llmCfg, "apiKey"))
	baseURL := strings.TrimSpace(readString(llmCfg, "baseURL"))
	if apiKey == "" && baseURL == "" {
		return nil
	}
	if !strings.Contains(model, "/") {
		model = "openai/" + model
	}
	mc := &config.ModelConfig{
		ModelName: "graph-memory-native-extract",
		Model:     model,
		APIKey:    apiKey,
		APIBase:   baseURL,
	}
	prov, modelID, err := providers.CreateProviderFromConfig(mc)
	if err != nil {
		return err
	}
	if sp, ok := prov.(providers.StatefulProvider); ok {
		defer sp.Close()
	}

	texts := make([]string, 0, len(messages))
	for _, m := range messages {
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		if len(c) > 500 {
			c = c[:500]
		}
		texts = append(texts, fmt.Sprintf("[%s] %s", m.Role, c))
	}
	if len(texts) == 0 {
		return nil
	}
	maxNodes := graphMemoryLLMExtractMaxNodes(cfg)
	prompt := strings.Join([]string{
		"Extract high-signal memory nodes from the conversation.",
		"Return JSON array only. No markdown fences.",
		"Each item fields: type(TASK|SKILL|EVENT), name, description, content, relatedTo(optional).",
		fmt.Sprintf("Return at most %d items.", maxNodes),
		"Conversation:",
		strings.Join(texts, "\n"),
	}, "\n")
	resp, err := prov.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil, modelID, map[string]any{
		"max_tokens":  800,
		"temperature": 0.1,
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return err
	}
	raw := stripJSONFence(resp.Content)
	var nodes []llmExtractNode
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		return err
	}
	for i := range nodes {
		n := nodes[i]
		if strings.TrimSpace(n.Name) == "" || strings.TrimSpace(n.Type) == "" || strings.TrimSpace(n.Content) == "" {
			continue
		}
		node, _, err := graphmemory.UpsertNode(db, n.Type, n.Name, n.Description, n.Content, sessionKey)
		if err != nil || node == nil {
			continue
		}
		if strings.TrimSpace(n.RelatedTo) != "" {
			related, err := graphmemory.SearchNodes(db, n.RelatedTo, 1)
			if err == nil && len(related) > 0 && strings.EqualFold(n.Type, "SKILL") {
				_ = graphmemory.LinkUsedSkill(db, related[0].ID, node.ID, sessionKey)
			}
		}
	}
	return nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func graphMemorySidecarObject(cfg *config.Config, key string) map[string]any {
	if cfg == nil || cfg.GraphMemory == nil || cfg.GraphMemory.Raw == nil {
		return nil
	}
	v, ok := cfg.GraphMemory.Raw[key]
	if !ok {
		return nil
	}
	m, _ := v.(map[string]any)
	return m
}

func readString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// SessionEndGraphMemory emits a session_end equivalent hook.
// Node bridge: forwards to plugin event.
// Native: runs a final maintenance pass for the session.
func SessionEndGraphMemory(ctx context.Context, cfg *config.Config, host *ManagedPluginHost, sessionKey string) {
	if strings.TrimSpace(sessionKey) == "" || ctx.Err() != nil {
		return
	}
	if GraphMemoryUseNodeBridge(cfg) {
		if host == nil {
			return
		}
		_, err := host.ContextOp(ctx, map[string]any{
			"op":    "emit",
			"event": "session_end",
			"eventPayload": map[string]any{
				"sessionKey": sessionKey,
				"sessionId":  sessionKey,
			},
			"ctx": map[string]any{
				"sessionKey": sessionKey,
				"sessionId":  sessionKey,
			},
		})
		if err != nil {
			logger.WarnCF("plugins", "graph-memory session_end emit failed", map[string]any{
				"session_key": sessionKey,
				"error":       err.Error(),
			})
		}
		return
	}

	if !graphmemory.Enabled(cfg) {
		return
	}
	dbPath := graphmemory.ResolveDBPath(cfg)
	db, err := graphmemory.OpenDB(dbPath)
	if err != nil {
		logger.WarnCF("plugins", "graph-memory native session_end open db failed", map[string]any{"error": err.Error()})
		return
	}
	defer db.Close()
	updated, err := graphmemory.RecomputePageRankHeuristic(db)
	if err != nil {
		logger.WarnCF("plugins", "graph-memory native session_end maintain failed", map[string]any{"error": err.Error()})
		return
	}
	logger.InfoCF("plugins", "graph-memory native session_end maintain done", map[string]any{
		"session_key": sessionKey,
		"updated":     updated,
	})
}
