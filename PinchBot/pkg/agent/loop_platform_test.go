package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
	"github.com/sipeed/pinchbot/pkg/routing"
	"github.com/sipeed/pinchbot/pkg/tools"
)

type captureOptionsProvider struct {
	lastOptions map[string]any
}

type recordedProviderCall struct {
	model   string
	options map[string]any
	prompt  string
}

type recordedProviderResponse struct {
	resp *providers.LLMResponse
	err  error
}

type recordingOptionsProvider struct {
	responses []recordedProviderResponse
	calls     []recordedProviderCall
}

type asyncToolCallProvider struct {
	callCount int
}

type immediateAsyncTool struct{}

func cloneOptions(options map[string]any) map[string]any {
	copied := make(map[string]any, len(options))
	for key, value := range options {
		copied[key] = value
	}
	return copied
}

func (c *captureOptionsProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	c.lastOptions = cloneOptions(options)
	return &providers.LLMResponse{Content: "ok", FinishReason: "stop"}, nil
}

func (c *captureOptionsProvider) GetDefaultModel() string {
	return "gpt-4"
}

func (r *recordingOptionsProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	prompt := ""
	if len(messages) > 0 {
		prompt = messages[len(messages)-1].Content
	}
	r.calls = append(r.calls, recordedProviderCall{
		model:   model,
		options: cloneOptions(options),
		prompt:  prompt,
	})
	callIndex := len(r.calls) - 1
	if callIndex < len(r.responses) {
		return r.responses[callIndex].resp, r.responses[callIndex].err
	}
	return &providers.LLMResponse{Content: "ok", FinishReason: "stop"}, nil
}

func (r *recordingOptionsProvider) GetDefaultModel() string {
	return "official/default"
}

func (p *asyncToolCallProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	p.callCount++
	if p.callCount == 1 {
		return &providers.LLMResponse{
			ToolCalls: []providers.ToolCall{
				{
					ID:        "call-1",
					Name:      "async_echo",
					Arguments: map[string]any{},
					Function: &providers.FunctionCall{
						Name:      "async_echo",
						Arguments: "{}",
					},
				},
			},
			FinishReason: "tool_calls",
		}, nil
	}
	return &providers.LLMResponse{Content: "done", FinishReason: "stop"}, nil
}

func (p *asyncToolCallProvider) GetDefaultModel() string {
	return "official/default"
}

func (immediateAsyncTool) Name() string {
	return "async_echo"
}

func (immediateAsyncTool) Description() string {
	return "Publishes an async completion immediately for tests."
}

func (immediateAsyncTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (immediateAsyncTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.AsyncResult("started")
}

func (immediateAsyncTool) ExecuteAsync(
	ctx context.Context,
	args map[string]any,
	cb tools.AsyncCallback,
) *tools.ToolResult {
	if cb != nil {
		cb(ctx, tools.SilentResult("async result"))
	}
	return tools.AsyncResult("started")
}

func officialCallContext(agent *AgentInstance) llmCallContext {
	return llmCallContext{
		candidates: []providers.FallbackCandidate{
			{Provider: "official", Model: "official-basic"},
		},
		model:           agent.Model,
		useToolProvider: false,
	}
}

func officialToolCallContext(agent *AgentInstance) llmCallContext {
	return llmCallContext{
		candidates: []providers.FallbackCandidate{
			{Provider: "official", Model: "official-tool"},
		},
		model:           agent.ToolModel,
		useToolProvider: true,
	}
}

func TestProcessMessagePropagatesPlatformAccessTokenForOfficialProvider(t *testing.T) {
	provider := &captureOptionsProvider{}
	cfg := testCfg(nil)
	cfg.Agents.Defaults.Model = "official/default"

	loop := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	_, err := loop.processMessage(context.Background(), bus.InboundMessage{
		Channel:    "launcher",
		SenderID:   "launcher",
		ChatID:     "chat-1",
		Content:    "hello",
		SessionKey: "launcher:chat-1",
		Metadata: map[string]string{
			"platform_access_token": "session-token",
		},
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if got := provider.lastOptions["user_access_token"]; got != "session-token" {
		t.Fatalf("user_access_token = %#v, want %q", got, "session-token")
	}
}

func TestProcessMessageDoesNotPropagatePlatformAccessTokenForNonOfficialProvider(t *testing.T) {
	provider := &captureOptionsProvider{}
	cfg := testCfg(nil)
	cfg.Agents.Defaults.Model = "gpt-4"

	loop := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	_, err := loop.processMessage(context.Background(), bus.InboundMessage{
		Channel:    "launcher",
		SenderID:   "launcher",
		ChatID:     "chat-1",
		Content:    "hello",
		SessionKey: "launcher:chat-1",
		Metadata: map[string]string{
			"platform_access_token": "session-token",
		},
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if _, ok := provider.lastOptions["user_access_token"]; ok {
		t.Fatalf("user_access_token unexpectedly set for non-official provider: %#v", provider.lastOptions["user_access_token"])
	}
}

func TestRetryLLMCallUsesOfficialToolCallContext(t *testing.T) {
	mainProvider := &recordingOptionsProvider{}
	toolProvider := &recordingOptionsProvider{
		responses: []recordedProviderResponse{
			{err: errors.New("temporary upstream failure")},
			{resp: &providers.LLMResponse{Content: "summary", FinishReason: "stop"}},
		},
	}
	cfg := testCfg(nil)
	cfg.Agents.Defaults.Model = "gpt-4"

	loop := NewAgentLoop(cfg, bus.NewMessageBus(), mainProvider)
	agent := loop.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	agent.ToolProvider = toolProvider
	agent.ToolModel = "official/tool"
	agent.ToolModelID = "official-tool"
	agent.ToolCandidates = []providers.FallbackCandidate{
		{Provider: "official", Model: "official-tool"},
	}

	resp, err := loop.retryLLMCall(
		context.Background(),
		agent,
		officialToolCallContext(agent),
		"summarize this",
		2,
		"session-token",
	)
	if err != nil {
		t.Fatalf("retryLLMCall() error = %v", err)
	}
	if resp == nil || resp.Content != "summary" {
		t.Fatalf("retryLLMCall() response = %#v, want summary content", resp)
	}
	if len(mainProvider.calls) != 0 {
		t.Fatalf("main provider calls = %d, want 0", len(mainProvider.calls))
	}
	if len(toolProvider.calls) != 2 {
		t.Fatalf("tool provider calls = %d, want 2", len(toolProvider.calls))
	}
	for callIndex, call := range toolProvider.calls {
		if got := call.options["user_access_token"]; got != "session-token" {
			t.Fatalf("call %d user_access_token = %#v, want %q", callIndex, got, "session-token")
		}
		if call.model != "official-tool" {
			t.Fatalf("call %d model = %q, want %q", callIndex, call.model, "official-tool")
		}
	}
}

func TestSummarizeSessionPropagatesPlatformAccessToken(t *testing.T) {
	mainProvider := &recordingOptionsProvider{}
	toolProvider := &recordingOptionsProvider{
		responses: []recordedProviderResponse{
			{resp: &providers.LLMResponse{Content: "part-one", FinishReason: "stop"}},
			{resp: &providers.LLMResponse{Content: "part-two", FinishReason: "stop"}},
			{resp: &providers.LLMResponse{Content: "merged", FinishReason: "stop"}},
		},
	}
	cfg := testCfg(nil)
	cfg.Agents.Defaults.Model = "gpt-4"

	loop := NewAgentLoop(cfg, bus.NewMessageBus(), mainProvider)
	agent := loop.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	agent.ToolProvider = toolProvider
	agent.ToolModel = "official/tool"
	agent.ToolModelID = "official-tool"
	agent.ToolCandidates = []providers.FallbackCandidate{
		{Provider: "official", Model: "official-tool"},
	}

	sessionKey := "launcher:chat-1"
	for i := 0; i < 16; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		agent.Sessions.AddFullMessage(sessionKey, providers.Message{
			Role:    role,
			Content: fmt.Sprintf("message-%02d", i),
		})
	}

	loop.summarizeSession(agent, sessionKey, officialToolCallContext(agent), "session-token")

	if got := agent.Sessions.GetSummary(sessionKey); got != "merged" {
		t.Fatalf("summary = %q, want %q", got, "merged")
	}
	if got := len(agent.Sessions.GetHistory(sessionKey)); got != 4 {
		t.Fatalf("remaining history = %d, want 4", got)
	}
	if len(mainProvider.calls) != 0 {
		t.Fatalf("main provider calls = %d, want 0", len(mainProvider.calls))
	}
	if len(toolProvider.calls) != 3 {
		t.Fatalf("tool provider calls = %d, want 3", len(toolProvider.calls))
	}
	for callIndex, call := range toolProvider.calls {
		if got := call.options["user_access_token"]; got != "session-token" {
			t.Fatalf("call %d user_access_token = %#v, want %q", callIndex, got, "session-token")
		}
		if call.model != "official-tool" {
			t.Fatalf("call %d model = %q, want %q", callIndex, call.model, "official-tool")
		}
	}
	if toolProvider.calls[2].prompt == "" {
		t.Fatal("expected merge prompt to be recorded")
	}
}

func TestAsyncToolPublishesSystemMessageWithPlatformToken(t *testing.T) {
	msgBus := bus.NewMessageBus()
	provider := &asyncToolCallProvider{}
	cfg := testCfg([]config.AgentConfig{
		{ID: "main", Default: true},
		{ID: "sales"},
	})
	cfg.Agents.Defaults.Model = "official/default"

	loop := NewAgentLoop(cfg, msgBus, provider)
	loop.RegisterTool(immediateAsyncTool{})
	salesAgent, ok := loop.registry.GetAgent("sales")
	if !ok || salesAgent == nil {
		t.Fatal("expected sales agent")
	}
	mainAgent := loop.registry.GetDefaultAgent()
	if mainAgent == nil {
		t.Fatal("expected default agent")
	}
	salesSessionKey := routing.BuildAgentMainSessionKey("sales")
	mainSessionKey := routing.BuildAgentMainSessionKey(mainAgent.ID)
	mainBefore := len(mainAgent.Sessions.GetHistory(mainSessionKey))
	salesBefore := len(salesAgent.Sessions.GetHistory(salesSessionKey))

	_, err := loop.runAgentLoop(context.Background(), salesAgent, processOptions{
		SessionKey:          salesSessionKey,
		Channel:             "launcher",
		ChatID:              "chat-1",
		UserMessage:         "do async work",
		PlatformAccessToken: "session-token",
		DefaultResponse:     defaultResponse,
		EnableSummary:       false,
		SendResponse:        false,
	})
	if err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	inbound, ok := msgBus.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected async system message")
	}
	if inbound.Channel != "system" {
		t.Fatalf("async inbound channel = %q, want %q", inbound.Channel, "system")
	}
	if inbound.SessionKey != salesSessionKey {
		t.Fatalf("async inbound sessionKey = %q, want %q", inbound.SessionKey, salesSessionKey)
	}
	if got := inbound.Metadata["platform_access_token"]; got != "session-token" {
		t.Fatalf("platform_access_token = %#v, want %q", got, "session-token")
	}
	if _, err := loop.processMessage(context.Background(), inbound); err != nil {
		t.Fatalf("processing async system message error = %v", err)
	}
	if got := len(mainAgent.Sessions.GetHistory(mainSessionKey)); got != mainBefore {
		t.Fatalf("default agent history len = %d, want unchanged %d", got, mainBefore)
	}
	if got := len(salesAgent.Sessions.GetHistory(salesSessionKey)); got <= salesBefore {
		t.Fatalf("sales agent history len = %d, want greater than %d", got, salesBefore)
	}
}
