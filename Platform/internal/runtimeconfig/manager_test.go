package runtimeconfig

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	if state.WalletSettings.MinRechargeAmountFen != 10 {
		t.Fatalf("wallet settings = %#v, want default minimum recharge amount 10 fen", state.WalletSettings)
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
	if normalized.WalletSettings.MinRechargeAmountFen != 10 {
		t.Fatalf("wallet settings = %#v, want default minimum recharge amount 10 fen", normalized.WalletSettings)
	}
}

func TestNormalizeModelsOrdersByFallbackPriority(t *testing.T) {
	models := []service.OfficialModel{
		{ID: "official-z", Name: "Z", Enabled: true, FallbackPriority: 20},
		{ID: "official-a", Name: "A", Enabled: true, FallbackPriority: 5},
		{ID: "official-b", Name: "B", Enabled: true, FallbackPriority: 5},
		{ID: "official-neg", Name: "NEG", Enabled: true, FallbackPriority: -3},
	}

	normalized := normalizeModels(models, nil, nil, 0)
	if len(normalized) != 4 {
		t.Fatalf("normalizeModels() count = %d, want 4", len(normalized))
	}

	wantIDs := []string{"official-neg", "official-a", "official-b", "official-z"}
	for i, want := range wantIDs {
		if normalized[i].ID != want {
			t.Fatalf("normalizeModels()[%d].ID = %q, want %q", i, normalized[i].ID, want)
		}
	}

	if normalized[0].FallbackPriority != 0 {
		t.Fatalf("normalized negative fallback_priority = %d, want 0", normalized[0].FallbackPriority)
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

func TestManagerSaveRoutesWithRevisionPreservesRedactedEndpoints(t *testing.T) {
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
					APIBase:   "https://user:pass@example.com/v1?token=abc",
					Proxy:     "http://proxy-user:proxy-pass@8.8.8.8:8080",
				},
			},
		},
	}); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	redactedRoutes := manager.RedactedSnapshot().OfficialRoutes
	revision, err := RevisionForRoutes(manager.Snapshot().OfficialRoutes)
	if err != nil {
		t.Fatalf("RevisionForRoutes() error = %v", err)
	}
	redactedRoutes[0].ModelConfig.Model = "openai/gpt-4.1-mini"
	if err := manager.SaveRoutesWithRevision(revision, redactedRoutes); err != nil {
		t.Fatalf("SaveRoutesWithRevision() error = %v", err)
	}

	snapshot := manager.Snapshot()
	if got := snapshot.OfficialRoutes[0].ModelConfig.APIBase; got != "https://user:pass@example.com/v1?token=abc" {
		t.Fatalf("stored api_base = %q, want original endpoint preserved", got)
	}
	if got := snapshot.OfficialRoutes[0].ModelConfig.Proxy; got != "http://proxy-user:proxy-pass@8.8.8.8:8080" {
		t.Fatalf("stored proxy = %q, want original proxy preserved", got)
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

func TestNormalizeStateRejectsUnresolvedOfficialRouteHost(t *testing.T) {
	previousLookup := lookupOfficialRouteHostIPs
	lookupOfficialRouteHostIPs = func(host string) ([]net.IP, error) {
		return nil, errors.New("dns failure")
	}
	t.Cleanup(func() {
		lookupOfficialRouteHostIPs = previousLookup
	})

	_, err := normalizeState(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "https://gateway.example.com/v1",
					APIKey:    "secret",
				},
			},
		},
		PricingRules: []service.PricingRule{
			{ModelID: "official-basic", FallbackPriceFen: 8},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "could not resolve") {
		t.Fatalf("normalizeState() error = %v, want unresolved-host validation failure", err)
	}
}

func TestNormalizeStatePreservesExplicitEmptyOfficialModels(t *testing.T) {
	normalized, err := normalizeState(State{
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
		OfficialModels: []service.OfficialModel{},
		PricingRules: []service.PricingRule{
			{ModelID: "official-basic", FallbackPriceFen: 8},
		},
	})
	if err != nil {
		t.Fatalf("normalizeState() error = %v", err)
	}
	if len(normalized.OfficialModels) != 0 {
		t.Fatalf("official models = %#v, want explicit empty catalog to stay empty", normalized.OfficialModels)
	}
}

func TestNormalizeStateRejectsEnabledModelWithoutPricingRule(t *testing.T) {
	_, err := normalizeState(State{
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
		OfficialModels: []service.OfficialModel{
			{
				ID:      "official-basic",
				Name:    "官方基础模型",
				Enabled: true,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "pricing") {
		t.Fatalf("normalizeState() error = %v, want missing-pricing validation failure", err)
	}
}

func TestRedactStateRemovesCredentialsFromOfficialRouteEndpoints(t *testing.T) {
	redacted := RedactState(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIKey:    "secret-key",
					APIBase:   "https://user:pass@example.com/v1?token=abc",
					Proxy:     "http://proxy-user:proxy-pass@proxy.example.com:8080",
				},
			},
		},
	})

	route := redacted.OfficialRoutes[0]
	if strings.Contains(route.ModelConfig.APIBase, "user:pass") || strings.Contains(route.ModelConfig.APIBase, "token=") {
		t.Fatalf("redacted api_base = %q, want credentials removed", route.ModelConfig.APIBase)
	}
	if strings.Contains(route.ModelConfig.Proxy, "proxy-user") || strings.Contains(route.ModelConfig.Proxy, "proxy-pass") {
		t.Fatalf("redacted proxy = %q, want credentials removed", route.ModelConfig.Proxy)
	}
}

func TestNormalizeStateRejectsCLIBackedOfficialRoute(t *testing.T) {
	_, err := normalizeState(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "claude-cli/sonnet",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("normalizeState() error = nil, want official route protocol rejection")
	}
}

func TestNormalizeStateRejectsLoopbackAPIBaseForOfficialRoute(t *testing.T) {
	_, err := normalizeState(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "http://127.0.0.1:4000/v1",
					APIKey:    "secret",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("normalizeState() error = nil, want loopback api_base rejection")
	}
}

func TestNormalizeStateRejectsHostnameResolvingToPrivateAddress(t *testing.T) {
	originalLookup := lookupOfficialRouteHostIPs
	lookupOfficialRouteHostIPs = func(host string) ([]net.IP, error) {
		if host == "gateway.example.com" {
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		}
		return nil, nil
	}
	defer func() { lookupOfficialRouteHostIPs = originalLookup }()

	_, err := normalizeState(State{
		OfficialRoutes: []upstream.OfficialRoute{
			{
				PublicModelID: "official-basic",
				ModelConfig: picocfg.ModelConfig{
					ModelName: "official-basic",
					Model:     "openai/gpt-4o-mini",
					APIBase:   "https://gateway.example.com/v1",
					APIKey:    "secret",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("normalizeState() error = nil, want hostname private-resolution rejection")
	}
}
