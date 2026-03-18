package launcherui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func writeLauncherUITestConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestNewHandlerServesEmbeddedIndex(t *testing.T) {
	cfgPath := writeLauncherUITestConfig(t, config.DefaultConfig())

	handler, err := NewHandler(cfgPath)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "账户与钱包") {
		t.Fatal("expected embedded launcher UI HTML in root response")
	}
}

func TestNewHandlerRegistersConfigAPI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{{ModelName: "gpt4", Model: "openai/gpt-5.2"}}
	cfgPath := writeLauncherUITestConfig(t, cfg)

	handler, err := NewHandler(cfgPath)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp struct {
		Config config.Config `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Config.ModelList) != 1 || resp.Config.ModelList[0].ModelName != "gpt4" {
		t.Fatalf("response config = %#v, want persisted config data", resp.Config.ModelList)
	}
}

func TestSyncOfficialModelsIntoConfigRemovesBootstrapSamplesButKeepsCustomModels(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "gpt-5.2",
			Model:     "openai/gpt-5.2",
			APIBase:   "https://api.openai.com/v1",
		},
		{
			ModelName: "my-custom-model",
			Model:     "openai/custom-model",
			APIBase:   "https://example.com/v1",
			APIKey:    "sk-real-custom-key",
		},
	}
	cfg.Agents.Defaults.ModelName = "gpt-5.2"
	cfgPath := writeLauncherUITestConfig(t, cfg)

	result, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "gpt-5.2", Name: "GPT-5.2 官方", Enabled: true},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if result.Removed < 1 {
		t.Fatalf("sync result removed = %d, want bootstrap sample removal", result.Removed)
	}
	if len(updated.ModelList) != 2 {
		t.Fatalf("updated model list len = %d, want 2 (official + custom)", len(updated.ModelList))
	}

	var hasOfficial bool
	var hasCustom bool
	for _, item := range updated.ModelList {
		switch item.ModelName {
		case "official-gpt-5.2":
			hasOfficial = item.Model == "official/gpt-5.2"
		case "my-custom-model":
			hasCustom = item.Model == "openai/custom-model"
		case "gpt-5.2":
			t.Fatalf("bootstrap sample model should have been removed, got %#v", item)
		}
	}
	if !hasOfficial {
		t.Fatal("expected synced official model to be present after removing bootstrap samples")
	}
	if !hasCustom {
		t.Fatal("expected custom model to remain after official model sync")
	}
	if updated.Agents.Defaults.ModelName != "official-gpt-5.2" {
		t.Fatalf("default model = %q, want %q after removing bootstrap sample", updated.Agents.Defaults.ModelName, "official-gpt-5.2")
	}
}
