package runtimeconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	picocfg "github.com/sipeed/pinchbot/pkg/config"

	platformconfig "openclaw/platform/internal/config"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/upstream"
)

func TestBuildStateFromEnvDerivesModelsFromRoutes(t *testing.T) {
	state, err := BuildStateFromEnv(platformconfig.Config{
		OfficialRoutesJSON: `[{"public_model_id":"official-basic","model_config":{"model":"openai/gpt-4o-mini","api_key":"secret"}}]`,
		PricingRulesJSON:   `[{"model_id":"official-basic","fallback_price_fen":8}]`,
		AgreementsJSON:     `[{"key":"recharge","version":"v1","title":"Recharge Notice","content":"Funds are used for official calls","url":"https://example.com/recharge"}]`,
	})
	if err != nil {
		t.Fatalf("BuildStateFromEnv() error = %v", err)
	}
	if len(state.OfficialRoutes) != 1 {
		t.Fatalf("routes = %d, want 1", len(state.OfficialRoutes))
	}
	if len(state.OfficialModels) != 1 {
		t.Fatalf("models = %d, want 1", len(state.OfficialModels))
	}
	if state.OfficialModels[0].ID != "official-basic" || !state.OfficialModels[0].Enabled {
		t.Fatalf("derived model = %#v, want enabled official-basic", state.OfficialModels[0])
	}
	if state.OfficialRoutes[0].ModelConfig.ModelName != "official-basic" {
		t.Fatalf("model_name = %q, want %q", state.OfficialRoutes[0].ModelConfig.ModelName, "official-basic")
	}
	if len(state.Agreements) != 1 || state.Agreements[0].Content == "" || state.Agreements[0].URL == "" {
		t.Fatalf("agreements = %#v, want content and url preserved", state.Agreements)
	}
}

func TestManagerBootstrapAppliesAndPersistsState(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "platform.runtime.json")
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	router := upstream.NewRouter(nil)
	manager := NewManager(path, svc, router)

	seed := State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret",
				},
			},
		},
		PricingRules: []service.PricingRule{
			{ModelID: "official-basic", FallbackPriceFen: 8},
		},
		Agreements: []service.AgreementDocument{
			{Key: "recharge", Version: "v1", Title: "Recharge Notice"},
		},
	}
	if err := manager.Bootstrap(seed); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("runtime config file missing: %v", err)
	}
	if len(router.Routes()) != 1 {
		t.Fatalf("router routes = %d, want 1", len(router.Routes()))
	}
	if len(svc.ListOfficialModels(nil)) != 1 {
		t.Fatalf("service models = %d, want 1", len(svc.ListOfficialModels(nil)))
	}
	if len(svc.ListPricingRules()) != 1 {
		t.Fatalf("service pricing rules = %d, want 1", len(svc.ListPricingRules()))
	}
	if len(svc.ListAgreements(nil)) != 1 {
		t.Fatalf("service agreements = %d, want 1", len(svc.ListAgreements(nil)))
	}
}

func TestRuntimeConfigExampleContainsNonEmptyOfficialModelFlow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "runtime-config.example.json"))
	if err != nil {
		t.Fatalf("ReadFile(runtime-config.example.json) error = %v", err)
	}

	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("Unmarshal(runtime-config.example.json) error = %v", err)
	}
	normalized, err := normalizeState(state)
	if err != nil {
		t.Fatalf("normalizeState(runtime-config.example.json) error = %v", err)
	}

	if len(normalized.OfficialRoutes) == 0 || len(normalized.OfficialModels) == 0 {
		t.Fatalf("normalized state = %#v, want at least one official route/model", normalized)
	}
	if normalized.OfficialModels[0].PricingVersion == "" {
		t.Fatalf("official model = %#v, want pricing_version for desktop sync", normalized.OfficialModels[0])
	}
	if len(normalized.PricingRules) == 0 || normalized.PricingRules[0].Version == "" || normalized.PricingRules[0].EffectiveFromUnix == 0 {
		t.Fatalf("pricing rules = %#v, want versioned pricing metadata", normalized.PricingRules)
	}
	if len(normalized.Agreements) < 3 {
		t.Fatalf("agreements = %#v, want user terms, privacy, and recharge agreements", normalized.Agreements)
	}
}

func TestManagerSaveRoutesWithRevisionPreservesRedactedSecrets(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "platform.runtime.json")
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	router := upstream.NewRouter(nil)
	manager := NewManager(path, svc, router)

	if err := manager.Bootstrap(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-key",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	redactedRoutes := manager.RedactedSnapshot().OfficialRoutes
	revision, err := RevisionForRoutes(manager.Snapshot().OfficialRoutes)
	if err != nil {
		t.Fatalf("ForPayload() error = %v", err)
	}
	redactedRoutes[0].ModelConfig.Model = "openai/gpt-4.1-mini"
	if err := manager.SaveRoutesWithRevision(revision, redactedRoutes); err != nil {
		t.Fatalf("SaveRoutesWithRevision() error = %v", err)
	}

	snapshot := manager.Snapshot()
	if got := snapshot.OfficialRoutes[0].ModelConfig.APIKey; got != "secret-key" {
		t.Fatalf("stored api_key = %q, want original secret preserved", got)
	}
	if got := snapshot.OfficialRoutes[0].ModelConfig.Model; got != "openai/gpt-4.1-mini" {
		t.Fatalf("stored model = %q, want updated model", got)
	}
}

func TestManagerSaveRoutesWithRevisionRejectsStaleRevisionAfterSecretRotation(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "platform.runtime.json")
	store := service.NewMemoryStore()
	svc := service.NewService(store, nil)
	router := upstream.NewRouter(nil)
	manager := NewManager(path, svc, router)

	if err := manager.Bootstrap(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-a",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	revision, err := RevisionForRoutes(manager.Snapshot().OfficialRoutes)
	if err != nil {
		t.Fatalf("RevisionForRoutes() error = %v", err)
	}
	if err := manager.SaveRoutesWithRevision("", []upstream.OfficialRoute{
		{
			PublicModelID: "official-basic",
			ModelConfig: picocfg.ModelConfig{
				ModelName: "official-basic",
				Model:     "openai/gpt-4o-mini",
				APIKey:    "secret-b",
			},
		},
	}); err != nil {
		t.Fatalf("SaveRoutesWithRevision(secret rotation) error = %v", err)
	}

	redactedRoutes := manager.RedactedSnapshot().OfficialRoutes
	redactedRoutes[0].ModelConfig.Model = "openai/gpt-4.1-mini"
	err = manager.SaveRoutesWithRevision(revision, redactedRoutes)
	if !errors.Is(err, service.ErrRevisionConflict) {
		t.Fatalf("SaveRoutesWithRevision() error = %v, want ErrRevisionConflict", err)
	}
}
