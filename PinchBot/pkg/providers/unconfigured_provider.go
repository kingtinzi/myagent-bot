package providers

import (
	"context"
	"fmt"
)

const unconfiguredModelGuidance = "尚未配置可用模型，请先在设置中选择官方模型或配置第三方模型后再发起聊天。"

// UnconfiguredProvider keeps the gateway process healthy even when the user has
// not selected any default model yet. It returns actionable guidance at chat
// time instead of crashing the whole gateway during startup.
type UnconfiguredProvider struct{}

func (p *UnconfiguredProvider) Chat(
	_ context.Context,
	_ []Message,
	_ []ToolDefinition,
	_ string,
	_ map[string]any,
) (*LLMResponse, error) {
	return nil, fmt.Errorf(unconfiguredModelGuidance)
}

func (p *UnconfiguredProvider) GetDefaultModel() string {
	return ""
}
