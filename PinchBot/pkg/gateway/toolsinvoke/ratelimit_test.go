package toolsinvoke

import (
	"net/http/httptest"
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestGatewayRateLimitFromConfig(t *testing.T) {
	if n := GatewayRateLimitFromConfig(nil); n != 0 {
		t.Fatalf("got %d", n)
	}
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			RateLimit: &config.GatewayRateLimitConfig{RequestsPerMinute: 30},
		},
	}
	if GatewayRateLimitFromConfig(cfg) != 30 {
		t.Fatal()
	}
}

func TestRateLimitExceeded_BearerKey(t *testing.T) {
	tok := "rate-limit-test-" + t.Name()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			RateLimit: &config.GatewayRateLimitConfig{RequestsPerMinute: 2},
		},
	}
	r := httptest.NewRequest("POST", "/tools/invoke", nil)
	r.RemoteAddr = "127.0.0.1:9"
	if RateLimitExceeded(cfg, r, tok) {
		t.Fatal("1")
	}
	if RateLimitExceeded(cfg, r, tok) {
		t.Fatal("2")
	}
	if !RateLimitExceeded(cfg, r, tok) {
		t.Fatal("3 should limit")
	}
}

func TestRateLimitExceeded_IPKey(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			RateLimit: &config.GatewayRateLimitConfig{RequestsPerMinute: 1},
		},
	}
	r := httptest.NewRequest("POST", "/tools/invoke", nil)
	r.RemoteAddr = "203.0.113." + "44" + ":5555"
	if RateLimitExceeded(cfg, r, "") {
		t.Fatal("first")
	}
	if !RateLimitExceeded(cfg, r, "") {
		t.Fatal("second should limit")
	}
}
