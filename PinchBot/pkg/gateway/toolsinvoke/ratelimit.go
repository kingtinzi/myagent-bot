package toolsinvoke

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
)

// GatewayRateLimitFromConfig returns requests per minute (0 = disabled).
func GatewayRateLimitFromConfig(cfg *config.Config) int {
	if cfg == nil || cfg.Gateway.RateLimit == nil {
		return 0
	}
	n := cfg.Gateway.RateLimit.RequestsPerMinute
	if n < 0 {
		return 0
	}
	return n
}

// gatewayRateLimiter is shared by /tools/invoke, /plugins/status, /plugins/gateway-method, and plugin registerHttpRoute paths so a client cannot bypass by switching path.
var gatewayRateLimiter = newMinuteWindowLimiter()

type minuteWindowLimiter struct {
	entries sync.Map // string -> *minuteCounter
}

type minuteCounter struct {
	mu          sync.Mutex
	count       int
	windowStart time.Time
}

func newMinuteWindowLimiter() *minuteWindowLimiter {
	return &minuteWindowLimiter{}
}

func (l *minuteWindowLimiter) allow(key string, limit int, now time.Time) bool {
	if limit <= 0 || key == "" {
		return true
	}
	v, _ := l.entries.LoadOrStore(key, &minuteCounter{})
	c := v.(*minuteCounter)
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Sub(c.windowStart) >= time.Minute {
		c.windowStart = now
		c.count = 0
	}
	if c.count >= limit {
		return false
	}
	c.count++
	return true
}

// RateLimitExceeded returns true if the request should be rejected with HTTP 429.
func RateLimitExceeded(cfg *config.Config, r *http.Request, bearer string) bool {
	n := GatewayRateLimitFromConfig(cfg)
	if n <= 0 || r == nil {
		return false
	}
	key := rateLimitClientKey(r, bearer)
	return !gatewayRateLimiter.allow(key, n, time.Now())
}

func rateLimitClientKey(r *http.Request, bearer string) string {
	b := strings.TrimSpace(bearer)
	if b != "" {
		sum := sha256.Sum256([]byte(b))
		return "bearer:" + hex.EncodeToString(sum[:12])
	}
	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	if x := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); x != "" {
		parts := strings.Split(x, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	ra := strings.TrimSpace(r.RemoteAddr)
	if ra == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(ra)
	if err != nil {
		return ra
	}
	return host
}

// WriteRateLimitJSON writes a 429 OpenClaw-style body and Retry-After.
func WriteRateLimitJSON(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	writeInvokeJSON(w, http.StatusTooManyRequests, map[string]any{
		"ok": false,
		"error": map[string]any{
			"type":    "rate_limited",
			"message": "too many requests; try again later",
		},
	})
}
