package config

import "testing"

func TestLoadFromEnvDefaultsAddrToLoopback(t *testing.T) {
	t.Setenv("PLATFORM_ADDR", "")

	cfg := LoadFromEnv()
	if cfg.Addr != "127.0.0.1:18791" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:18791")
	}
}

func TestLoadFromEnvHonorsExplicitAddr(t *testing.T) {
	t.Setenv("PLATFORM_ADDR", "0.0.0.0:29999")

	cfg := LoadFromEnv()
	if cfg.Addr != "0.0.0.0:29999" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, "0.0.0.0:29999")
	}
}

func TestLoadFromEnvUsesPublishableKeyAliasWhenAnonKeyMissing(t *testing.T) {
	t.Setenv("PLATFORM_SUPABASE_ANON_KEY", "")
	t.Setenv("PLATFORM_SUPABASE_PUBLISHABLE_KEY", "publishable-key")

	cfg := LoadFromEnv()
	if cfg.SupabaseAnonKey != "publishable-key" {
		t.Fatalf("SupabaseAnonKey = %q, want %q", cfg.SupabaseAnonKey, "publishable-key")
	}
}

func TestLoadFromEnvPrefersAnonKeyWhenBothAliasAndAnonKeyAreSet(t *testing.T) {
	t.Setenv("PLATFORM_SUPABASE_ANON_KEY", "anon-key")
	t.Setenv("PLATFORM_SUPABASE_PUBLISHABLE_KEY", "publishable-key")

	cfg := LoadFromEnv()
	if cfg.SupabaseAnonKey != "anon-key" {
		t.Fatalf("SupabaseAnonKey = %q, want %q", cfg.SupabaseAnonKey, "anon-key")
	}
}

func TestLoadFromEnvReadsPrimaryFailureRedisSettings(t *testing.T) {
	t.Setenv("PLATFORM_PRIMARY_FAILURE_REDIS_URL", "redis://:secret@127.0.0.1:6379/9")
	t.Setenv("PLATFORM_PRIMARY_FAILURE_REDIS_KEY_PREFIX", "platform:test:breaker")

	cfg := LoadFromEnv()
	if cfg.PrimaryFailureRedisURL != "redis://:secret@127.0.0.1:6379/9" {
		t.Fatalf("PrimaryFailureRedisURL = %q, want %q", cfg.PrimaryFailureRedisURL, "redis://:secret@127.0.0.1:6379/9")
	}
	if cfg.PrimaryFailureRedisKeyPrefix != "platform:test:breaker" {
		t.Fatalf("PrimaryFailureRedisKeyPrefix = %q, want %q", cfg.PrimaryFailureRedisKeyPrefix, "platform:test:breaker")
	}
}
