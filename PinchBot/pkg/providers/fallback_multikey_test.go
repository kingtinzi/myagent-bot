package providers

import (
	"context"
	"errors"
	"testing"
)

func TestMultiKeyFailover_SucceedsOnSecondKey(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "glm-4.7",
		Fallbacks: []string{"glm-4.7__key_1", "glm-4.7__key_2"},
	}
	candidates := ResolveCandidates(cfg, "zhipu")
	if len(candidates) != 3 {
		t.Fatalf("len(candidates) = %d, want 3", len(candidates))
	}

	chain := NewFallbackChain(NewCooldownTracker())
	attempt := 0
	resp, err := chain.Execute(context.Background(), candidates, func(
		_ context.Context,
		_ string,
		_ string,
	) (*LLMResponse, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("status: 429")
		}
		return &LLMResponse{Content: "ok"}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp == nil || resp.Response == nil || resp.Response.Content != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if attempt != 2 {
		t.Fatalf("attempt = %d, want 2", attempt)
	}
}

func TestMultiKeyFailover_CooldownSkipsOnlyOneKey(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "glm-4.7",
		Fallbacks: []string{"glm-4.7__key_1"},
	}
	candidates := ResolveCandidates(cfg, "zhipu")
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(candidates))
	}

	ct := NewCooldownTracker()
	firstKey := ModelKey(candidates[0].Provider, candidates[0].Model)
	ct.MarkFailure(firstKey, FailoverRateLimit)

	chain := NewFallbackChain(ct)
	calls := 0
	resp, err := chain.Execute(context.Background(), candidates, func(
		_ context.Context,
		_ string,
		_ string,
	) (*LLMResponse, error) {
		calls++
		return &LLMResponse{Content: "fallback-ok"}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if len(resp.Attempts) != 1 || !resp.Attempts[0].Skipped {
		t.Fatalf("attempts = %+v, want one skipped attempt", resp.Attempts)
	}
}

func TestMultiKeyThenModelFallback(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "glm-4.7",
		Fallbacks: []string{"glm-4.7__key_1", "minimax/minimax"},
	}
	candidates := ResolveCandidates(cfg, "zhipu")
	if len(candidates) != 3 {
		t.Fatalf("len(candidates) = %d, want 3", len(candidates))
	}

	chain := NewFallbackChain(NewCooldownTracker())
	attempt := 0
	resp, err := chain.Execute(context.Background(), candidates, func(
		_ context.Context,
		_ string,
		_ string,
	) (*LLMResponse, error) {
		attempt++
		if attempt <= 2 {
			return nil, errors.New("status: 429")
		}
		return &LLMResponse{Content: "from-minimax"}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp.Response.Content != "from-minimax" {
		t.Fatalf("content = %q, want from-minimax", resp.Response.Content)
	}
}
