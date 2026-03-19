package feishu

import (
	"context"
	"sync"
	"time"
)

type tokenCache struct {
	mu    sync.RWMutex
	store map[string]*tokenEntry
}

type tokenEntry struct {
	value    string
	expireAt time.Time
}

func newTokenCache() *tokenCache {
	return &tokenCache{
		store: make(map[string]*tokenEntry),
	}
}

func (c *tokenCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = &tokenEntry{
		value:    value,
		expireAt: time.Now().Add(ttl),
	}
	return nil
}

func (c *tokenCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.store[key]
	if !ok {
		return "", nil
	}
	if time.Now().After(entry.expireAt) {
		delete(c.store, key)
		return "", nil
	}
	return entry.value, nil
}

func (c *tokenCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.store)
}
