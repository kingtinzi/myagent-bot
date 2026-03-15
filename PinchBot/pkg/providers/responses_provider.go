package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const defaultResponsesAPIBase = "https://api.openai.com/v1"

type ResponsesProvider struct {
	client          *openai.Client
	enableWebSearch bool
}

func NewResponsesProvider(token, baseURL string) *ResponsesProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultResponsesAPIBase
	}
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(strings.TrimSpace(token)),
	)
	return &ResponsesProvider{
		client:          &client,
		enableWebSearch: false,
	}
}

func (p *ResponsesProvider) Chat(
	ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]any,
) (*LLMResponse, error) {
	resolvedModel := strings.TrimSpace(model)
	if resolvedModel == "" {
		resolvedModel = codexDefaultModel
	}

	params := buildCodexParams(messages, tools, resolvedModel, options, p.enableWebSearch)
	stream := p.client.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var resp *responses.Response
	for stream.Next() {
		evt := stream.Current()
		if evt.Type == "response.completed" || evt.Type == "response.failed" || evt.Type == "response.incomplete" {
			evtResp := evt.Response
			resp = &evtResp
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("responses stream failed: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("responses stream finished without terminal response")
	}

	result := parseCodexResponse(resp)
	return result, nil
}

func (p *ResponsesProvider) GetDefaultModel() string {
	return codexDefaultModel
}
