package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"
	"github.com/sipeed/pinchbot/pkg/providers/protocoltypes"
)

const defaultOfficialAPIBase = "http://142.91.105.49:18793"

type OfficialProvider struct {
	baseURL string
	client  *http.Client
}

func NewOfficialProvider(baseURL string) *OfficialProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultOfficialAPIBase
	}
	return &OfficialProvider{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OfficialProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	token, _ := options["user_access_token"].(string)
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("official model requires user_access_token")
	}

	requestOptions := make(map[string]any, len(options))
	for key, value := range options {
		if key == "user_access_token" {
			continue
		}
		requestOptions[key] = value
	}

	body, err := json.Marshal(platformapi.ChatProxyRequest{
		ModelID:  model,
		Messages: toProtocolMessages(messages),
		Tools:    toProtocolTools(tools),
		Options:  requestOptions,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/official", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		return nil, &platformapi.APIError{
			StatusCode: resp.StatusCode,
			Message:    message,
		}
	}
	var out platformapi.ChatProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	result := LLMResponse(out.Response)
	return &result, nil
}

func (p *OfficialProvider) GetDefaultModel() string {
	return "official/default"
}

func toProtocolMessages(messages []Message) []protocoltypes.Message {
	out := make([]protocoltypes.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, protocoltypes.Message(msg))
	}
	return out
}

func toProtocolTools(tools []ToolDefinition) []protocoltypes.ToolDefinition {
	out := make([]protocoltypes.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, protocoltypes.ToolDefinition(tool))
	}
	return out
}
