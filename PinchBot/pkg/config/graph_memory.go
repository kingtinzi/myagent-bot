package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const graphMemoryConfigEnv = "PINCHBOT_GRAPH_MEMORY_CONFIG"

// GraphMemoryFileConfig is loaded from config.graph-memory.json (sidecar), not from config.json.
type GraphMemoryFileConfig struct {
	Enabled bool           `json:"enabled"`
	Raw     map[string]any `json:"-"` // full document for Node pluginConfig
}

// ResolveGraphMemoryConfigPath returns the path to the graph-memory sidecar file.
func ResolveGraphMemoryConfigPath(mainConfigPath string) string {
	if env := strings.TrimSpace(os.Getenv(graphMemoryConfigEnv)); env != "" {
		return expandHome(env)
	}
	mainConfigPath = strings.TrimSpace(mainConfigPath)
	if mainConfigPath == "" {
		return filepath.Join(GetPinchBotHome(), "config.graph-memory.json")
	}
	return filepath.Join(filepath.Dir(mainConfigPath), "config.graph-memory.json")
}

// LoadGraphMemorySidecar reads config.graph-memory.json next to the main config (or PINCHBOT_GRAPH_MEMORY_CONFIG).
// When the file is missing or enabled is false, returns (nil, nil).
func LoadGraphMemorySidecar(mainConfigPath string) (*GraphMemoryFileConfig, error) {
	path := ResolveGraphMemoryConfigPath(mainConfigPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	enabled := false
	switch v := raw["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	case float64:
		enabled = v != 0
	}
	if !enabled {
		return nil, nil
	}
	return &GraphMemoryFileConfig{Enabled: true, Raw: raw}, nil
}
