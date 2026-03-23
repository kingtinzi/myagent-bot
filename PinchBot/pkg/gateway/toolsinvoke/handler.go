// Package toolsinvoke implements POST /tools/invoke for OpenClaw-compatible tool automation (e.g. Lobster openclaw.invoke).
// Gateway auth and the optional per-minute rate limiter are shared with GET /plugins/status, POST /plugins/gateway-method, and plugin HTTP routes (see ratelimit.go).
package toolsinvoke

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/routing"
	"github.com/sipeed/pinchbot/pkg/tools"
)

const maxBodyBytes = 2 * 1024 * 1024

// Handler serves POST /tools/invoke on the Gateway mux.
type Handler struct {
	Cfg         *config.Config
	AgentReg    func() *agent.AgentRegistry
	HTTPChannel string // e.g. "http-invoke"; used as ToolChannel context
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeInvokeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "method_not_allowed",
				"message": "POST required",
			},
		})
		return
	}

	ch := strings.TrimSpace(h.HTTPChannel)
	if ch == "" {
		ch = "http-invoke"
	}

	bearer := BearerFromRequest(r)
	if RateLimitExceeded(h.Cfg, r, bearer) {
		WriteRateLimitJSON(w)
		return
	}
	if !CheckGatewayInvokeAuth(h.Cfg, bearer) {
		writeInvokeJSON(w, http.StatusUnauthorized, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "unauthorized",
				"message": "invalid or missing gateway credentials",
			},
		})
		return
	}

	body, err := readLimitedJSONBody(r, maxBodyBytes)
	if err != nil {
		writeInvokeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "invalid_request",
				"message": err.Error(),
			},
		})
		return
	}

	var req struct {
		Tool       string         `json:"tool"`
		Action     string         `json:"action"`
		Args       map[string]any `json:"args"`
		SessionKey string         `json:"sessionKey"`
		DryRun     bool           `json:"dryRun"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeInvokeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "invalid_request",
				"message": "invalid JSON body",
			},
		})
		return
	}

	toolName := strings.TrimSpace(req.Tool)
	if toolName == "" {
		writeInvokeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "invalid_request",
				"message": "tools.invoke requires body.tool",
			},
		})
		return
	}

	if IsGatewayHTTPDenied(h.Cfg, toolName) {
		writeInvokeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "not_found",
				"message": fmt.Sprintf("Tool not available: %s", toolName),
			},
		})
		return
	}

	reg := h.AgentReg()
	if reg == nil {
		writeInvokeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "tool_error",
				"message": "agent registry unavailable",
			},
		})
		return
	}

	ag := resolveAgentForInvoke(reg, req.SessionKey)
	if ag == nil {
		writeInvokeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "not_found",
				"message": "no agent for sessionKey",
			},
		})
		return
	}

	if config.DeniedByAgentToolsProfile(agent.ResolvedAgentToolsProfile(h.Cfg, ag.ID), toolName) {
		writeInvokeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "not_found",
				"message": fmt.Sprintf("Tool not available: %s", toolName),
			},
		})
		return
	}

	toolInst, ok := ag.Tools.Get(toolName)
	if !ok {
		writeInvokeJSON(w, http.StatusNotFound, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "not_found",
				"message": fmt.Sprintf("Tool not available: %s", toolName),
			},
		})
		return
	}

	action := strings.TrimSpace(req.Action)
	args := req.Args
	if args == nil {
		args = map[string]any{}
	}
	merged := MergeToolInvokeArgs(toolInst.Parameters(), action, args)

	msgChannel := ch
	if v := strings.TrimSpace(r.Header.Get("x-openclaw-message-channel")); v != "" {
		msgChannel = v
	}
	chatID := strings.TrimSpace(r.Header.Get("x-openclaw-thread-id"))
	if chatID == "" {
		chatID = "invoke"
	}

	if req.DryRun {
		logger.InfoCF("gateway", "tools/invoke dryRun",
			map[string]any{"tool": toolName, "agent": ag.ID, "sessionKey": strings.TrimSpace(req.SessionKey)})
		writeInvokeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"dryRun": true,
			"result": map[string]any{
				"content": fmt.Sprintf(
					"Dry run: would invoke tool %q on agent %q (not executed).",
					toolName,
					ag.ID,
				),
				"for_llm":  "",
				"is_error": false,
			},
			"resolved": map[string]any{
				"agent_id":    ag.ID,
				"tool":        toolName,
				"merged_args": merged,
				"session_key": strings.TrimSpace(req.SessionKey),
				"channel":     msgChannel,
				"chat_id":     chatID,
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	ctx = tools.WithAgentID(ctx, ag.ID)
	ctx = tools.WithToolContext(ctx, msgChannel, chatID)

	logger.InfoCF("gateway", "tools/invoke",
		map[string]any{"tool": toolName, "agent": ag.ID, "sessionKey": strings.TrimSpace(req.SessionKey)})

	result := ag.Tools.ExecuteWithContext(ctx, toolName, merged, msgChannel, chatID, nil)
	if result == nil {
		writeInvokeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "tool_error",
				"message": "tool execution failed",
			},
		})
		return
	}
	if result.IsError {
		msg := strings.TrimSpace(result.ForUser)
		if msg == "" {
			msg = strings.TrimSpace(result.ForLLM)
		}
		writeInvokeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":    "tool_error",
				"message": msg,
			},
		})
		return
	}

	writeInvokeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": toolInvokeResultPayload(result),
	})
}

func resolveAgentForInvoke(reg *agent.AgentRegistry, sessionKey string) *agent.AgentInstance {
	sk := strings.TrimSpace(sessionKey)
	if sk == "" || strings.EqualFold(sk, routing.DefaultMainKey) {
		return reg.GetDefaultAgent()
	}
	if parsed := routing.ParseAgentSessionKey(sk); parsed != nil {
		if ag, ok := reg.GetAgent(parsed.AgentID); ok {
			return ag
		}
	}
	return reg.GetDefaultAgent()
}

func readLimitedJSONBody(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("empty body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, limit+1))
	dec.UseNumber()
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("body too large (max %d bytes)", limit)
	}
	return raw, nil
}

func writeInvokeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func toolInvokeResultPayload(tr *tools.ToolResult) any {
	if tr == nil {
		return nil
	}
	content := strings.TrimSpace(tr.ForUser)
	if content == "" {
		content = tr.ForLLM
	}
	return map[string]any{
		"content":  content,
		"for_llm":  tr.ForLLM,
		"is_error": tr.IsError,
	}
}
