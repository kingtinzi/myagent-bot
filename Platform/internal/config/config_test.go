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
