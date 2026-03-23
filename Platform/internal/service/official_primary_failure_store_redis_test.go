package service

import "testing"

func TestNewRedisOfficialPrimaryFailureStoreRejectsInvalidURL(t *testing.T) {
	_, err := NewRedisOfficialPrimaryFailureStore(RedisOfficialPrimaryFailureStoreConfig{
		URL: "://bad-url",
	})
	if err == nil {
		t.Fatal("expected error for invalid redis url")
	}
}

func TestNewRedisOfficialPrimaryFailureStoreParsesSettings(t *testing.T) {
	store, err := NewRedisOfficialPrimaryFailureStore(RedisOfficialPrimaryFailureStoreConfig{
		URL:       "redis://user:secret@127.0.0.1:6380/7",
		KeyPrefix: "platform:test:breaker",
	})
	if err != nil {
		t.Fatalf("NewRedisOfficialPrimaryFailureStore() error = %v", err)
	}
	redisStore, ok := store.(*redisOfficialPrimaryFailureStore)
	if !ok {
		t.Fatalf("store type = %T, want *redisOfficialPrimaryFailureStore", store)
	}
	if redisStore.addr != "127.0.0.1:6380" {
		t.Fatalf("addr = %q, want %q", redisStore.addr, "127.0.0.1:6380")
	}
	if redisStore.username != "user" || redisStore.password != "secret" {
		t.Fatalf("credentials = %q/%q, want user/secret", redisStore.username, redisStore.password)
	}
	if redisStore.db != 7 {
		t.Fatalf("db = %d, want 7", redisStore.db)
	}
	if redisStore.keyPrefix != "platform:test:breaker:" {
		t.Fatalf("keyPrefix = %q, want %q", redisStore.keyPrefix, "platform:test:breaker:")
	}
}

func TestNormalizeRedisPrimaryFailureKeyPrefixDefaults(t *testing.T) {
	if got := normalizeRedisPrimaryFailureKeyPrefix(""); got != defaultRedisPrimaryFailureKeyPrefix {
		t.Fatalf("normalizeRedisPrimaryFailureKeyPrefix(\"\") = %q, want %q", got, defaultRedisPrimaryFailureKeyPrefix)
	}
	if got := normalizeRedisPrimaryFailureKeyPrefix("platform:x"); got != "platform:x:" {
		t.Fatalf("normalizeRedisPrimaryFailureKeyPrefix(\"platform:x\") = %q, want %q", got, "platform:x:")
	}
}
