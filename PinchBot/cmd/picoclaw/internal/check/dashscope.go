package check

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sipeed/pinchbot/cmd/picoclaw/internal"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/providers"
)

func newDashScopeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashscope",
		Short: "Test DashScope (Qwen) API connectivity",
		RunE:  runDashScopeCheck,
	}
	return cmd
}

// findDashScopeModelConfig returns a ModelConfig that uses DashScope (api_base contains dashscope).
// It checks model_list first, then falls back to providers.qwen.
func findDashScopeModelConfig(cfg *config.Config) (*config.ModelConfig, error) {
	for i := range cfg.ModelList {
		m := &cfg.ModelList[i]
		if strings.Contains(strings.ToLower(m.APIBase), "dashscope") {
			return m, nil
		}
	}
	// Fallback: use legacy providers.qwen
	q := &cfg.Providers.Qwen
	if q.APIKey == "" && q.APIBase == "" {
		return nil, fmt.Errorf("no DashScope config found: add a model with api_base containing 'dashscope' in model_list, or set providers.qwen.api_key and api_base")
	}
	apiBase := q.APIBase
	if apiBase == "" {
		apiBase = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return &config.ModelConfig{
		Model:          "qwen/qwen3-14b",
		ModelName:      "qwen3-14b",
		APIBase:        apiBase,
		APIKey:         q.APIKey,
		Proxy:          q.Proxy,
		RequestTimeout: q.RequestTimeout,
	}, nil
}

func runDashScopeCheck(_ *cobra.Command, _ []string) error {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	mc, err := findDashScopeModelConfig(cfg)
	if err != nil {
		return err
	}

	provider, modelID, err := providers.CreateProviderFromConfig(mc)
	if err != nil {
		return fmt.Errorf("create DashScope provider: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []providers.Message{
		{Role: "user", Content: "hi"},
	}
	opts := map[string]any{"max_tokens": 10}

	fmt.Println("Testing DashScope connection...")
	_, err = provider.Chat(ctx, messages, nil, modelID, opts)
	if err != nil {
		return fmt.Errorf("DashScope connection failed: %w", err)
	}

	fmt.Println("✓ DashScope connection OK")
	return nil
}
