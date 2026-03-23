// Package pluginsrepair serves POST /plugins/repair (M3 skeleton).
// Authorization and rate limiting match pkg/gateway/toolsinvoke.
package pluginsrepair

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/gateway/toolsinvoke"
	"github.com/sipeed/pinchbot/pkg/plugins"
)

const maxBodyBytes = 2 * 1024 * 1024

var (
	resolvePluginRootFn = resolvePluginRoot
	installNodeDepsFn   = installNodeDeps
	installBundledCliFn = installBundledCLI
)

type Handler struct {
	Cfg      *config.Config
	AgentReg func() *agent.AgentRegistry
}

type requestBody struct {
	PluginID string         `json:"plugin_id"`
	ActionID string         `json:"action_id"`
	Args     map[string]any `json:"args"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "POST required"})
		return
	}
	if h == nil || h.AgentReg == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	bearer := toolsinvoke.BearerFromRequest(r)
	if toolsinvoke.RateLimitExceeded(h.Cfg, r, bearer) {
		toolsinvoke.WriteRateLimitJSON(w)
		return
	}
	if !toolsinvoke.CheckGatewayInvokeAuth(h.Cfg, bearer) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	_ = r.Body.Close()

	var req requestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid JSON"})
		return
	}
	req.PluginID = strings.ToLower(strings.TrimSpace(req.PluginID))
	req.ActionID = strings.ToLower(strings.TrimSpace(req.ActionID))
	if req.PluginID == "" || req.ActionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plugin_id and action_id are required"})
		return
	}
	switch req.ActionID {
	case "explain_fix", "set_env_path_hint":
		explain := explainFix(req.PluginID, req.ActionID)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"status": "ok",
			"result": map[string]any{
				"plugin_id": req.PluginID,
				"action_id": req.ActionID,
				"mode":      "explain_only",
				"message":   explain,
			},
		})
		return
	case "install_bundled_cli":
		approve, _ := req.Args["approve"].(bool)
		if !approve {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":     true,
				"status": "needs_approval",
				"approval": map[string]any{
					"token":   "plugins-repair-approval",
					"title":   "确认安装扩展运行时",
					"message": "将安装 Lobster CLI 到可执行环境（npm global），并验证 `lobster --version`。",
				},
			})
			return
		}
		mode, out, runErr := installBundledCliFn(r.Context(), req.PluginID)
		if runErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok":     false,
				"status": "failed",
				"error":  runErr.Error(),
				"result": map[string]any{
					"plugin_id": req.PluginID,
					"action_id": req.ActionID,
					"mode":      mode,
					"logs":      strings.TrimSpace(out),
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"status": "ok",
			"result": map[string]any{
				"plugin_id": req.PluginID,
				"action_id": req.ActionID,
				"mode":      mode,
				"message":   "Lobster CLI 安装完成，请刷新扩展诊断查看最新状态。",
				"logs":      strings.TrimSpace(out),
			},
		})
		return
	case "install_node_deps":
		reg := h.AgentReg()
		if reg == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "no registry"})
			return
		}
		ag := reg.GetDefaultAgent()
		if ag == nil || strings.TrimSpace(ag.Workspace) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "default agent workspace unavailable"})
			return
		}
		root, err := resolvePluginRootFn(ag.Workspace, h.Cfg, req.PluginID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		mode, out, runErr := installNodeDepsFn(r.Context(), root)
		if runErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok":     false,
				"status": "failed",
				"error":  runErr.Error(),
				"result": map[string]any{
					"plugin_id": req.PluginID,
					"action_id": req.ActionID,
					"mode":      mode,
					"logs":      strings.TrimSpace(out),
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"status": "ok",
			"result": map[string]any{
				"plugin_id": req.PluginID,
				"action_id": req.ActionID,
				"mode":      mode,
				"message":   "依赖安装完成，请刷新扩展诊断查看最新状态。",
				"logs":      strings.TrimSpace(out),
			},
		})
		return
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "unsupported action_id"})
		return
	}
}

func explainFix(pluginID, actionID string) string {
	if pluginID == "lobster" {
		switch actionID {
		case "install_bundled_cli":
			return "建议动作：安装内置 Lobster 运行时到应用私有目录（不修改系统全局 PATH）。安装后再次校验 lobster --version。"
		case "set_env_path_hint":
			return "手动修复：设置 LOBSTER_BIN 指向可执行入口，或确保 lobster 在 PATH 可见；重启网关后刷新扩展诊断。"
		case "explain_fix":
			return "说明模式：先确认 node/npm/lobster 三项检查结果，再执行安装或环境变量修复。"
		}
	}
	return "该修复动作尚未接入自动执行。请参考诊断信息进行手动修复。"
}

func resolvePluginRoot(workspace string, cfg *config.Config, pluginID string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("workspace is empty")
	}
	extRel := ""
	if cfg != nil {
		extRel = cfg.Plugins.ExtensionsDir
	}
	extDir, err := plugins.ResolveExtensionsDir(workspace, extRel)
	if err != nil {
		return "", err
	}
	discovered, err := plugins.DiscoverEnabled(extDir, []string{pluginID})
	if err != nil {
		return "", err
	}
	if len(discovered) == 0 {
		return "", fmt.Errorf("plugin %s is not discoverable under %s", pluginID, extDir)
	}
	return discovered[0].Root, nil
}

func installNodeDeps(parentCtx context.Context, pluginRoot string) (mode string, output string, err error) {
	lockPath := filepath.Join(pluginRoot, "package-lock.json")
	mode = "npm install"
	args := []string{"install"}
	if st, statErr := os.Stat(lockPath); statErr == nil && !st.IsDir() {
		mode = "npm ci"
		args = []string{"ci"}
	}
	npmPath, lookErr := exec.LookPath("npm")
	if lookErr != nil {
		return mode, "", fmt.Errorf("npm not found in PATH")
	}
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, npmPath, args...)
	cmd.Dir = pluginRoot
	raw, runErr := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return mode, string(raw), fmt.Errorf("%s timed out", mode)
	}
	if runErr != nil {
		return mode, string(raw), fmt.Errorf("%s failed: %w", mode, runErr)
	}
	return mode, string(raw), nil
}

func installBundledCLI(parentCtx context.Context, pluginID string) (mode string, output string, err error) {
	pluginID = strings.ToLower(strings.TrimSpace(pluginID))
	if pluginID != "lobster" {
		return "unsupported", "", fmt.Errorf("install_bundled_cli is not supported for plugin %s", pluginID)
	}
	npmPath, lookErr := exec.LookPath("npm")
	if lookErr != nil {
		return "npm install -g @clawdbot/lobster", "", fmt.Errorf("npm not found in PATH")
	}
	ctx, cancel := context.WithTimeout(parentCtx, 3*time.Minute)
	defer cancel()

	mode = "npm install -g @clawdbot/lobster"
	installCmd := exec.CommandContext(ctx, npmPath, "install", "-g", "@clawdbot/lobster")
	rawInstall, installErr := installCmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return mode, string(rawInstall), fmt.Errorf("%s timed out", mode)
	}
	if installErr != nil {
		return mode, string(rawInstall), fmt.Errorf("%s failed: %w", mode, installErr)
	}

	verifyCmd := exec.CommandContext(ctx, "lobster", "--version")
	rawVerify, verifyErr := verifyCmd.CombinedOutput()
	out := strings.TrimSpace(string(rawInstall))
	verify := strings.TrimSpace(string(rawVerify))
	if verify != "" {
		if out != "" {
			out += "\n"
		}
		out += verify
	}
	if verifyErr != nil {
		return mode, out, fmt.Errorf("lobster verification failed: %w", verifyErr)
	}
	return mode, out, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

