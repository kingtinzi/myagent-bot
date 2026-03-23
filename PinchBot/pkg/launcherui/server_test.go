package launcherui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
		case "official":
			hasOfficial = item.Model == canonicalOfficialModelRef
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
	if updated.Agents.Defaults.ModelName != "official" {
		t.Fatalf("default model = %q, want %q after removing bootstrap sample", updated.Agents.Defaults.ModelName, "official")
	}
}

func TestSyncOfficialModelsIntoConfigBuildsSingleOfficialWithoutLocalFallbackRefs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "my-custom-model",
			Model:     "openai/custom-model",
			APIBase:   "https://example.com/v1",
			APIKey:    "sk-real-custom-key",
		},
	}
	cfg.Agents.Defaults.ModelName = "my-custom-model"
	cfgPath := writeLauncherUITestConfig(t, cfg)

	_, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "alpha", Name: "Alpha", Enabled: true},
		{ID: "beta", Name: "Beta", Enabled: true},
		{ID: "gamma", Name: "Gamma", Enabled: true},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(updated.ModelList) != 2 {
		t.Fatalf("updated model list len = %d, want 2 (official + custom)", len(updated.ModelList))
	}
	assertModelTarget(t, updated.ModelList, "official", canonicalOfficialModelRef)
	assertModelFallbacks(t, updated.ModelList, "official", nil)
	for _, item := range updated.ModelList {
		if strings.HasPrefix(item.ModelName, "official-") {
			t.Fatalf("unexpected per-model official alias after sync: %#v", item)
		}
	}
}

func TestSyncOfficialModelsIntoConfigDoesNotExposePriorityFallbackRefs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "my-custom-model",
			Model:     "openai/custom-model",
			APIBase:   "https://example.com/v1",
			APIKey:    "sk-real-custom-key",
		},
	}
	cfgPath := writeLauncherUITestConfig(t, cfg)

	_, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "primary", Name: "Primary", Enabled: true, FallbackPriority: 0},
		{ID: "backup-b", Name: "BackupB", Enabled: true, FallbackPriority: 20},
		{ID: "backup-a", Name: "BackupA", Enabled: true, FallbackPriority: 10},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	assertModelTarget(t, updated.ModelList, "official", canonicalOfficialModelRef)
	assertModelFallbacks(t, updated.ModelList, "official", nil)
}

func TestSyncOfficialModelsIntoConfigReordersPrimaryByPriorityEvenWhenLegacyPrimaryExists(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "official",
			Model:     "official/alpha",
			APIBase:   "http://127.0.0.1:18793",
		},
	}
	cfg.Agents.Defaults.ModelName = "official"
	cfgPath := writeLauncherUITestConfig(t, cfg)

	_, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "alpha", Name: "Alpha", Enabled: true, FallbackPriority: 20},
		{ID: "beta", Name: "Beta", Enabled: true, FallbackPriority: 0},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	var canonical *config.ModelConfig
	for i := range updated.ModelList {
		if updated.ModelList[i].ModelName == "official" {
			canonical = &updated.ModelList[i]
			break
		}
	}
	if canonical == nil {
		t.Fatal("official canonical model not found")
	}
	if canonical.Model != canonicalOfficialModelRef {
		t.Fatalf("canonical official model = %q, want %q", canonical.Model, canonicalOfficialModelRef)
	}
	if canonical.Fallbacks != nil {
		t.Fatalf("canonical fallbacks = %v, want nil", canonical.Fallbacks)
	}
}

func TestSyncOfficialModelsIntoConfigCanonicalizesLegacyOfficialAliases(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "vip-main",
			Model:     "official/alpha",
			APIBase:   "http://127.0.0.1:18793",
			Fallbacks: []string{"legacy-fallback"},
		},
		{
			ModelName: "official-beta",
			Model:     "official/beta",
			APIBase:   "http://127.0.0.1:18793",
		},
	}
	cfg.Agents.Defaults.ModelName = "vip-main"
	cfgPath := writeLauncherUITestConfig(t, cfg)

	_, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "alpha", Name: "Alpha", Enabled: true},
		{ID: "beta", Name: "Beta", Enabled: true},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	officialCount := 0
	for _, item := range updated.ModelList {
		if _, ok := officialModelID(item.Model); ok {
			officialCount++
		}
	}
	if officialCount != 1 {
		t.Fatalf("official model count = %d, want 1", officialCount)
	}
	assertModelTarget(t, updated.ModelList, "official", canonicalOfficialModelRef)
	assertModelFallbacks(t, updated.ModelList, "official", nil)
	for _, item := range updated.ModelList {
		if item.ModelName == "vip-main" || item.ModelName == "official-beta" {
			t.Fatalf("legacy official alias should be canonicalized, got %#v", item)
		}
	}
}

func TestSyncOfficialModelsIntoConfigRenamesConflictingCustomOfficialAliasAndPreservesDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "official",
			Model:     "openai/custom-model",
			APIBase:   "https://example.com/v1",
			APIKey:    "sk-real-custom-key",
		},
		{
			ModelName: "official-custom",
			Model:     "openai/custom-model-2",
			APIBase:   "https://example.com/v1",
			APIKey:    "sk-real-custom-key-2",
		},
	}
	cfg.Agents.Defaults.ModelName = "official"
	cfgPath := writeLauncherUITestConfig(t, cfg)

	result, err := syncOfficialModelsIntoConfig(cfgPath, []platformapi.OfficialModel{
		{ID: "alpha", Name: "Alpha", Enabled: true},
	})
	if err != nil {
		t.Fatalf("syncOfficialModelsIntoConfig() error = %v", err)
	}

	updated, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(updated.ModelList) != 3 {
		t.Fatalf("updated model list len = %d, want 3 (official + renamed custom + existing custom)", len(updated.ModelList))
	}

	var (
		officialAliasCount int
		customMainRenamed  bool
		customOtherKept    bool
	)
	seenAliases := make(map[string]struct{}, len(updated.ModelList))
	for _, item := range updated.ModelList {
		alias := strings.ToLower(strings.TrimSpace(item.ModelName))
		if _, exists := seenAliases[alias]; exists {
			t.Fatalf("duplicate model alias found after sync: %q (model=%q)", item.ModelName, item.Model)
		}
		seenAliases[alias] = struct{}{}

		if item.ModelName == "official" {
			officialAliasCount++
			if item.Model != canonicalOfficialModelRef {
				t.Fatalf("official canonical model = %q, want %q", item.Model, canonicalOfficialModelRef)
			}
			if item.Fallbacks != nil {
				t.Fatalf("official canonical fallbacks = %v, want nil", item.Fallbacks)
			}
			continue
		}
		if item.Model == "openai/custom-model" {
			customMainRenamed = true
			if item.ModelName != "official-custom-1" {
				t.Fatalf("renamed custom alias = %q, want %q", item.ModelName, "official-custom-1")
			}
		}
		if item.Model == "openai/custom-model-2" {
			customOtherKept = true
			if item.ModelName != "official-custom" {
				t.Fatalf("existing custom alias = %q, want %q", item.ModelName, "official-custom")
			}
		}
	}
	if officialAliasCount != 1 {
		t.Fatalf("official alias count = %d, want 1", officialAliasCount)
	}
	if !customMainRenamed || !customOtherKept {
		t.Fatalf("expected both custom models to remain after alias conflict resolution, main=%v other=%v", customMainRenamed, customOtherKept)
	}
	if result.Updated < 1 {
		t.Fatalf("sync result updated = %d, want >= 1 after alias conflict resolution", result.Updated)
	}
	if !result.DefaultChanged || result.DefaultModel != "official-custom-1" {
		t.Fatalf("default change = %#v, want changed to renamed custom alias", result)
	}
	if updated.Agents.Defaults.GetModelName() != "official-custom-1" {
		t.Fatalf("default model = %q, want %q", updated.Agents.Defaults.GetModelName(), "official-custom-1")
	}
}

func TestBuildAppOfficialModelSummariesRedactsModelIdentity(t *testing.T) {
	summaries := buildAppOfficialModelSummaries([]platformapi.OfficialModel{
		{
			ID:             "gpt-5.2",
			Name:           "官方 GPT-5.2",
			Enabled:        true,
			PricingVersion: "v20260317",
		},
	})
	if len(summaries) != 1 {
		t.Fatalf("summary len = %d, want 1", len(summaries))
	}
	if !summaries[0].Enabled {
		t.Fatal("expected summary item to preserve enabled state")
	}
	if summaries[0].PricingSummary == "" {
		t.Fatal("expected summary item to retain safe pricing hint")
	}

	payload, err := json.Marshal(summaries[0])
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	raw := string(payload)
	for _, marker := range []string{`"id"`, `"name"`, `"description"`} {
		if strings.Contains(raw, marker) {
			t.Fatalf("summary payload leaked model identity field %q: %s", marker, raw)
		}
	}
}

func assertModelFallbacks(t *testing.T, models []config.ModelConfig, modelName string, expected []string) {
	t.Helper()
	for _, item := range models {
		if item.ModelName != modelName {
			continue
		}
		if !reflect.DeepEqual(item.Fallbacks, expected) {
			t.Fatalf("model %q fallbacks = %v, want %v", modelName, item.Fallbacks, expected)
		}
		return
	}
	t.Fatalf("model %q not found", modelName)
}

func assertModelTarget(t *testing.T, models []config.ModelConfig, modelName string, expected string) {
	t.Helper()
	for _, item := range models {
		if item.ModelName != modelName {
			continue
		}
		if item.Model != expected {
			t.Fatalf("model %q target = %q, want %q", modelName, item.Model, expected)
		}
		return
	}
	t.Fatalf("model %q not found", modelName)
}
