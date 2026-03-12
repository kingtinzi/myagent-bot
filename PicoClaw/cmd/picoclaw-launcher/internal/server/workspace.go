package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// expandWorkspacePath expands ~ to the user's home directory (~ or ~/rest or ~\rest).
func expandWorkspacePath(path string) (string, error) {
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
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(config.GetPinchBotHome(), path), nil
}

// RegisterWorkspaceAPI registers workspace-related API.
func RegisterWorkspaceAPI(mux *http.ServeMux) {
	// POST /api/workspace/init: ensure directory exists; PinchBot fills starter files when the gateway boots.
	mux.HandleFunc("POST /api/workspace/init", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		expanded, err := expandWorkspacePath(req.Path)
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
