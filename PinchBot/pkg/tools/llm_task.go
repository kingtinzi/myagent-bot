package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

const llmTaskSchemaURL = "https://pinchbot.local/llm-task-inline-schema.json"

// Match OpenClaw llm-task: optional ```json fence wrapping the whole output.
var llmTaskJSONFenceRe = regexp.MustCompile(`(?is)^` + "```" + `(?:json)?\s*(.*?)\s*` + "```" + `$`)

type llmTaskTool struct {
	cfg          *config.Config
	workspace    string
	agentID      string
	primaryModel string
}

// NewLlmTaskTool builds the native llm-task tool: one JSON-only LLM completion (no tools), optional JSON Schema validation.
func NewLlmTaskTool(cfg *config.Config, workspace, agentID, primaryModelName string) Tool {
	return &llmTaskTool{
		cfg:          cfg,
		workspace:    workspace,
		agentID:      agentID,
		primaryModel: strings.TrimSpace(primaryModelName),
	}
}

func (t *llmTaskTool) Name() string {
	return "llm-task"
}

func (t *llmTaskTool) Description() string {
	return "Run a generic JSON-only LLM task and return optional schema-validated JSON. No tools are available to the model for this call. Use together with other native tools via the agent allowlist."
}

func (t *llmTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Task instruction for the LLM.",
			},
			"input": map[string]any{
				"description": "Optional input payload for the task (any JSON-serializable value).",
			},
			"schema": map[string]any{
				"type":        "object",
				"description": "Optional JSON Schema object to validate the returned JSON.",
			},
			"provider": map[string]any{
				"type":        "string",
				"description": "Optional provider protocol override (e.g. openai, anthropic).",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model_list model_name override.",
			},
			"thinking": map[string]any{
				"type":        "string",
				"description": "Optional thinking level: off, low, medium, high, adaptive, xhigh (xhigh: Anthropic only).",
			},
			"authProfileId": map[string]any{
				"type":        "string",
				"description": "Reserved for OpenClaw parity; ignored by PinchBot.",
			},
			"temperature": map[string]any{
				"type":        "number",
				"description": "Optional sampling temperature.",
			},
			"maxTokens": map[string]any{
				"type":        "number",
				"description": "Optional max tokens for the completion.",
			},
			"timeoutMs": map[string]any{
				"type":        "number",
				"description": "Timeout for the LLM request in milliseconds.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *llmTaskTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.cfg == nil {
		return ErrorResult("llm-task: config not available")
	}
	prompt, _ := args["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return ErrorResult("prompt required")
	}

	plug := t.cfg.Plugins.LlmTask

	lookupName := strings.TrimSpace(strArg(args, "model"))
	if lookupName == "" && plug != nil {
		lookupName = strings.TrimSpace(plug.DefaultModel)
	}
	if lookupName == "" {
		lookupName = t.primaryModel
	}
	if lookupName == "" {
		return ErrorResult("could not resolve model (set agents model or pass model parameter)")
	}

	mc, err := t.cfg.GetModelConfig(lookupName)
	if err != nil || mc == nil {
		return ErrorResult(fmt.Sprintf("model %q not found in model_list", lookupName)).WithError(err)
	}

	mcWork := *mc
	fullModel := strings.TrimSpace(mcWork.Model)
	if fullModel == "" {
		return ErrorResult("model_list entry has empty model field")
	}
	provOverride := strings.TrimSpace(strArg(args, "provider"))
	if provOverride == "" && plug != nil {
		provOverride = strings.TrimSpace(plug.DefaultProvider)
	}
	if provOverride != "" {
		_, mid := providers.ExtractProtocol(fullModel)
		mid = strings.TrimSpace(mid)
		if mid == "" {
			return ErrorResult("cannot apply provider override: model id is empty")
		}
		mcWork.Model = providers.NormalizeProvider(provOverride) + "/" + mid
		fullModel = mcWork.Model
	}

	protocol, modelID := providers.ExtractProtocol(fullModel)
	protocol = providers.NormalizeProvider(protocol)
	modelID = strings.TrimSpace(modelID)
	if protocol == "" || modelID == "" {
		return ErrorResult("provider/model could not be resolved")
	}

	modelKey := protocol + "/" + modelID
	if allowed := llmTaskAllowedModels(plug); len(allowed) > 0 {
		if !allowedContains(allowed, modelKey) {
			return ErrorResult(fmt.Sprintf("model not allowed by plugins.llm_task.allowed_models: %s (allowed: %s)",
				modelKey, strings.Join(allowed, ", ")))
		}
	}

	thinkingRaw := strings.TrimSpace(strArg(args, "thinking"))
	if thinkingRaw == "xhigh" && !strings.EqualFold(protocol, "anthropic") {
		return ErrorResult(`thinking level "xhigh" is only supported for anthropic models in PinchBot`)
	}

	timeoutMs := intFromArg(args, "timeoutMs", 0)
	if timeoutMs <= 0 && plug != nil && plug.TimeoutMs > 0 {
		timeoutMs = plug.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 30_000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	llmProv, chatModelID, err := providers.CreateProviderFromConfig(&mcWork)
	if err != nil || llmProv == nil {
		return ErrorResult(fmt.Sprintf("llm-task: create provider: %v", err)).WithError(err)
	}
	if sp, ok := llmProv.(providers.StatefulProvider); ok {
		defer sp.Close()
	}
	chatModelID = strings.TrimSpace(chatModelID)
	if chatModelID == "" {
		chatModelID = modelID
	}

	inputJSON := "null"
	if _, has := args["input"]; has {
		b, err := json.MarshalIndent(args["input"], "", "  ")
		if err != nil {
			return ErrorResult("input must be JSON-serializable").WithError(err)
		}
		inputJSON = string(b)
	}

	fullPrompt := strings.Join([]string{
		"You are a JSON-only function.",
		"Return ONLY a valid JSON value.",
		"Do not wrap in markdown fences.",
		"Do not include commentary.",
		"Do not call tools.",
		"",
		"TASK:",
		strings.TrimSpace(prompt),
		"",
		"INPUT_JSON:",
		inputJSON,
	}, "\n")

	maxTok := intFromArg(args, "maxTokens", 0)
	if maxTok <= 0 && plug != nil && plug.MaxTokens > 0 {
		maxTok = plug.MaxTokens
	}
	if maxTok <= 0 {
		maxTok = t.cfg.Agents.Defaults.MaxTokens
		if maxTok <= 0 {
			maxTok = 8192
		}
	}

	temp := -1.0
	if v, ok := args["temperature"].(float64); ok {
		temp = v
	}
	if temp < 0 && t.cfg.Agents.Defaults.Temperature != nil {
		temp = *t.cfg.Agents.Defaults.Temperature
	}
	if temp < 0 {
		temp = 0.7
	}

	opts := map[string]any{
		"max_tokens":       maxTok,
		"temperature":      temp,
		"prompt_cache_key": "llm-task:" + t.agentID,
	}
	if thinkingRaw != "" {
		if tc, ok := llmProv.(providers.ThinkingCapable); ok && tc.SupportsThinking() {
			opts["thinking_level"] = thinkingRaw
		}
	}

	messages := []providers.Message{{Role: "user", Content: fullPrompt}}
	resp, err := llmProv.Chat(runCtx, messages, nil, chatModelID, opts)
	if err != nil {
		return ErrorResult(fmt.Sprintf("llm-task: %v", err)).WithError(err)
	}
	if resp == nil {
		return ErrorResult("LLM returned empty response")
	}
	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return ErrorResult("LLM returned empty output")
	}
	if len(resp.ToolCalls) > 0 {
		return ErrorResult("LLM returned tool calls; llm-task expects JSON text only")
	}

	raw := stripJSONCodeFences(text)
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ErrorResult("LLM returned invalid JSON").WithError(err)
	}

	if sch, ok := args["schema"].(map[string]any); ok && len(sch) > 0 {
		if err := validateJSONSchema(sch, parsed); err != nil {
			return ErrorResult(err.Error())
		}
	}

	out, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return ErrorResult("failed to format JSON result").WithError(err)
	}
	return SilentResult(string(out))
}

func strArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func intFromArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return def
	}
}

func llmTaskAllowedModels(plug *config.LlmTaskPluginConfig) []string {
	if plug == nil {
		return nil
	}
	var out []string
	for _, s := range plug.AllowedModels {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func allowedContains(allowed []string, key string) bool {
	k := strings.TrimSpace(key)
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), k) {
			return true
		}
	}
	return false
}

func stripJSONCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if m := llmTaskJSONFenceRe.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}

func validateJSONSchema(schema map[string]any, instance any) error {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(llmTaskSchemaURL, schema); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}
	sch, err := c.Compile(llmTaskSchemaURL)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := sch.Validate(instance); err != nil {
		return fmt.Errorf("LLM JSON did not match schema: %w", err)
	}
	return nil
}
