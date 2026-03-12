package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// resolveWorkspacePath 将配置中的工作区路径解析为绝对路径。
// 相对路径（如 "workspace"）相对于 GetPinchBotHome()（即 exe 同目录的 .pinchbot）；~ 表示用户家目录。
func resolveWorkspacePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if len(path) > 1 && path[0] == '~' && (path[1] == '/' || path[1] == filepath.Separator) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(GetPinchBotHome(), path), nil
	}
	return path, nil
}

// RegisterWorkspaceAPI registers workspace-related API (e.g. init placeholder for future use).
func RegisterWorkspaceAPI(mux *http.ServeMux) {
	// POST /api/workspace/init: ensure directory exists; actual template copy is done by "PinchBot onboard".
	mux.HandleFunc("POST /api/workspace/init", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		expanded, err := resolveWorkspacePath(req.Path)
		if err != nil {
			http.Error(w, "Failed to expand path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if expanded == "" {
			http.Error(w, "Path is required", http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(expanded, 0o755); err != nil {
			http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": expanded})
	})
}
