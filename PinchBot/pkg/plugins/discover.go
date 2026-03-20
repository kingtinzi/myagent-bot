// Package plugins discovers OpenClaw-style extensions and optionally loads them via a Node host.
package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const manifestName = "openclaw.plugin.json"

// DiscoveredPlugin is a resolved extension directory with manifest id.
type DiscoveredPlugin struct {
	ID           string
	Root         string
	PluginConfig map[string]any // optional; passed to Node register(api).pluginConfig
}

type rawManifest struct {
	ID string `json:"id"`
}

// DiscoverEnabled scans extensionsRoot for subdirectories containing openclaw.plugin.json.
// Only plugins whose manifest id matches an entry in enabled (case-insensitive, trimmed) are returned.
func DiscoverEnabled(extensionsRoot string, enabled []string) ([]DiscoveredPlugin, error) {
	if extensionsRoot == "" {
		return nil, fmt.Errorf("extensions root is empty")
	}
	absRoot, err := filepath.Abs(extensionsRoot)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("extensions dir: %w", err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("extensions path is not a directory: %s", absRoot)
	}

	want := make(map[string]struct{})
	for _, e := range enabled {
		k := strings.ToLower(strings.TrimSpace(e))
		if k != "" {
			want[k] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil, nil
	}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil, err
	}

	var out []DiscoveredPlugin
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		pluginDir := filepath.Join(absRoot, name)
		manifestPath := filepath.Join(pluginDir, manifestName)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m rawManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := want[strings.ToLower(id)]; !ok {
			continue
		}
		out = append(out, DiscoveredPlugin{ID: id, Root: pluginDir})
	}
	return out, nil
}

// ResolveExtensionsDir returns an absolute path: if rel is absolute, it is cleaned; otherwise joined with workspace.
func ResolveExtensionsDir(workspace, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "extensions"
	}
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel), nil
	}
	if workspace == "" {
		return "", fmt.Errorf("workspace is empty")
	}
	return filepath.Abs(filepath.Join(workspace, rel))
}
