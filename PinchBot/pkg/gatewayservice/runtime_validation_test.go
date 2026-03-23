package gatewayservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestValidateFeishuPluginMode_BuiltinPathNoWarnings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.UseBuiltinChannel = true

	var logs []string
	validateFeishuPluginMode(cfg, func(msg string) { logs = append(logs, msg) })
	if len(logs) != 0 {
		t.Fatalf("expected no warnings, got: %v", logs)
	}
}

func TestValidateFeishuPluginMode_MisconfiguredWarnings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = ""
	cfg.Channels.Feishu.AppSecret = ""
	cfg.Plugins.Enabled = []string{"graph-memory", config.OpenclawLarkPluginID}
	cfg.Plugins.NodeHost = false

	var logs []string
	validateFeishuPluginMode(cfg, func(msg string) { logs = append(logs, msg) })
	if len(logs) == 0 {
		t.Fatal("expected warnings for misconfigured plugin mode")
	}
	joined := strings.Join(logs, "\n")
	for _, want := range []string{
		"plugins.node_host=false",
		"app_id/app_secret",
		"Failed to discover enabled plugins",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected warning containing %q, got logs: %v", want, logs)
		}
	}
}

func TestValidateFeishuPluginMode_DiscoverablePluginNoWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PINCHBOT_HOME", home)

	extensionsDir := filepath.Join(home, "workspace", "extensions", config.OpenclawLarkPluginID)
	if err := os.MkdirAll(extensionsDir, 0o755); err != nil {
		t.Fatalf("mkdir extensions dir: %v", err)
	}
	manifest := `{"id":"openclaw-lark","configSchema":{"type":"object","properties":{}}}`
	if err := os.WriteFile(filepath.Join(extensionsDir, "openclaw.plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = "cli_xxx"
	cfg.Channels.Feishu.AppSecret = "secret_xxx"
	cfg.Plugins.NodeHost = true
	cfg.Plugins.Enabled = append(cfg.Plugins.Enabled, config.OpenclawLarkPluginID)

	var logs []string
	validateFeishuPluginMode(cfg, func(msg string) { logs = append(logs, msg) })
	if len(logs) != 0 {
		t.Fatalf("expected no warnings with discoverable plugin, got: %v", logs)
	}
}
