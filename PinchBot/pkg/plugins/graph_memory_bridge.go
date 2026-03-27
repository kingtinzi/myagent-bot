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

// graphMemoryNativeImplemented: graph-memory is implemented only in Go (pkg/graphmemory).
// The legacy Node/TypeScript extension and hooks are removed.
const graphMemoryNativeImplemented = true

// GraphMemoryRuntimeActive is true when sidecar + plugin enablement are ready for the Go runtime.
func GraphMemoryRuntimeActive(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		return false
	}
	if !cfg.Plugins.IsPluginEnabled(DefaultGraphMemoryEngineID) {
		return false
	}
	return graphMemoryNativeImplemented
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

// EmitGraphMemoryBeforeAgentStart is kept for lifecycle parity and is a no-op in Go-native runtime.
func EmitGraphMemoryBeforeAgentStart(ctx context.Context, cfg *config.Config, prompt, sessionKey string) {
	_ = ctx
	_ = cfg
	_ = prompt
	_ = sessionKey
	// no-op
}

// AssembleGraphMemory assembles Go-native memory context for current session.
func AssembleGraphMemory(
	ctx context.Context,
	cfg *config.Config,
	engineID, sessionKey string,
	messages []providers.Message,
	tokenBudget int,
) (systemAddition string, tail []providers.Message, err error) {
	_ = ctx
	_ = engineID
	_ = tokenBudget
	return assembleGraphMemoryNative(cfg, sessionKey, messages)
}

// AfterTurnGraphMemory ingests turn data and runs native extraction/maintenance.
func AfterTurnGraphMemory(
	ctx context.Context,
	cfg *config.Config,
	engineID, sessionKey string,
	messages []providers.Message,
	prePromptCount int,
	isHeartbeat bool,
) {
	if ctx.Err() != nil {
		return
	}
	_ = engineID
	afterTurnGraphMemoryNative(cfg, sessionKey, messages, prePromptCount, isHeartbeat)
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

// SessionEndGraphMemory runs a final native maintenance pass for the session.
func SessionEndGraphMemory(ctx context.Context, cfg *config.Config, sessionKey string) {
	if strings.TrimSpace(sessionKey) == "" || ctx.Err() != nil {
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
