package runtimeconfig

import (
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
