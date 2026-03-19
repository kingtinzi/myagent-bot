package feishu

import (
	"context"
	"testing"
	"time"
)

func TestTokenCache_GetSetAndExpiry(t *testing.T) {
	c := newTokenCache()
	if err := c.Set(context.Background(), "k", "v", 20*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("get before expiry: %v", err)
	}
	if got != "v" {
		t.Fatalf("expected value %q, got %q", "v", got)
	}

	time.Sleep(35 * time.Millisecond)
	got, err = c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("get after expiry: %v", err)
	}
	if got != "" {
		t.Fatalf("expected expired value to be empty, got %q", got)
	}
}

func TestTokenCache_InvalidateAll(t *testing.T) {
	c := newTokenCache()
	_ = c.Set(context.Background(), "a", "1", time.Minute)
	_ = c.Set(context.Background(), "b", "2", time.Minute)

	c.InvalidateAll()

	a, _ := c.Get(context.Background(), "a")
	b, _ := c.Get(context.Background(), "b")
	if a != "" || b != "" {
		t.Fatalf("expected all cache entries cleared, got a=%q b=%q", a, b)
	}
}
