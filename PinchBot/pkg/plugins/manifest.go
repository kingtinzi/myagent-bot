package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateOpenClawManifest checks openclaw.plugin.json for OpenClaw-style requirements:
// non-empty id and a present configSchema that is a JSON object (not null).
// Aligns with upstream loader behavior for extension manifests.
func ValidateOpenClawManifest(data []byte) (id string, err error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return "", fmt.Errorf("manifest JSON: %w", err)
	}
	if len(top) == 0 {
		return "", fmt.Errorf("manifest: empty object")
	}
	rawID, ok := top["id"]
	if !ok {
		return "", fmt.Errorf("manifest: missing id")
	}
	if err := json.Unmarshal(rawID, &id); err != nil {
		return "", fmt.Errorf("manifest id: %w", err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("manifest: id is empty")
	}
	rawCS, ok := top["configSchema"]
	if !ok {
		return id, fmt.Errorf("plugin %q: configSchema is required (OpenClaw parity)", id)
	}
	trimmed := bytes.TrimSpace(rawCS)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return id, fmt.Errorf("plugin %q: configSchema cannot be null or empty", id)
	}
	var cs map[string]json.RawMessage
	if err := json.Unmarshal(rawCS, &cs); err != nil || cs == nil {
		return id, fmt.Errorf("plugin %q: configSchema must be a JSON object", id)
	}
	return id, nil
}
