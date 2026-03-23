package toolsinvoke

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
)

// DefaultGatewayHTTPToolDeny matches OpenClaw gateway HTTP defaults (high-risk over non-interactive HTTP).
var DefaultGatewayHTTPToolDeny = []string{
	"sessions_spawn",
	"sessions_send",
	"cron",
	"gateway",
	"whatsapp_login",
}

// IsGatewayHTTPDenied returns true if the tool must not be callable via POST /tools/invoke.
func IsGatewayHTTPDenied(cfg *config.Config, toolName string) bool {
	n := strings.ToLower(strings.TrimSpace(toolName))
	if n == "" {
		return true
	}
	allowRemove := map[string]bool{}
	extraDeny := map[string]bool{}
	if cfg != nil && cfg.Gateway.Tools != nil {
		for _, a := range cfg.Gateway.Tools.Allow {
			allowRemove[strings.ToLower(strings.TrimSpace(a))] = true
		}
		for _, d := range cfg.Gateway.Tools.Deny {
			extraDeny[strings.ToLower(strings.TrimSpace(d))] = true
		}
	}
	if extraDeny[n] {
		return true
	}
	for _, d := range DefaultGatewayHTTPToolDeny {
		if strings.ToLower(strings.TrimSpace(d)) != n {
			continue
		}
		return !allowRemove[n]
	}
	return false
}

// BearerFromRequest extracts the Bearer credential from the Authorization header (trimmed), or empty.
func BearerFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	lower := strings.ToLower(h)
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// CheckGatewayInvokeAuth validates Bearer token for /tools/invoke.
// When auth is nil or mode is empty/none, allows all requests (backward compatible).
func CheckGatewayInvokeAuth(cfg *config.Config, bearer string) bool {
	if cfg == nil || cfg.Gateway.Auth == nil {
		return true
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Gateway.Auth.Mode))
	if mode == "" || mode == "none" {
		return true
	}
	secret := strings.TrimSpace(bearer)
	if secret == "" {
		return false
	}
	switch mode {
	case "token":
		want := strings.TrimSpace(cfg.Gateway.Auth.Token)
		if want == "" {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(secret), []byte(want)) == 1
	case "password":
		want := strings.TrimSpace(cfg.Gateway.Auth.Password)
		if want == "" {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(secret), []byte(want)) == 1
	default:
		return false
	}
}

// MergeToolInvokeArgs merges body.action into args when the tool schema exposes an "action" property
// and args does not already set "action" (OpenClaw-compatible).
func MergeToolInvokeArgs(toolSchema map[string]any, action string, args map[string]any) map[string]any {
	if action == "" {
		return args
	}
	if args != nil {
		if _, has := args["action"]; has {
			return args
		}
	}
	props, _ := toolSchema["properties"].(map[string]any)
	if props == nil {
		return args
	}
	if _, ok := props["action"]; !ok {
		return args
	}
	out := make(map[string]any)
	if args != nil {
		for k, v := range args {
			out[k] = v
		}
	}
	out["action"] = action
	return out
}
