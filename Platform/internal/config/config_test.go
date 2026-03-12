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
