package providers

import (
	"context"
	"strings"
	"testing"
)

func TestUnconfiguredProviderReturnsGuidanceForChat(t *testing.T) {
	provider := &UnconfiguredProvider{}

	_, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "你好"}}, nil, "", nil)
	if err == nil {
		t.Fatal("Chat() error = nil, want guidance for configuring a model")
	}
	message := err.Error()
	if !strings.Contains(message, "尚未配置可用模型") {
		t.Fatalf("error = %q, want model configuration guidance", message)
	}
	if !strings.Contains(message, "设置") && !strings.Contains(message, "官方模型") {
		t.Fatalf("error = %q, want actionable guidance for configuring the model source", message)
	}
}
