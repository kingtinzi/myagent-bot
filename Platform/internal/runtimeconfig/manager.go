package runtimeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	platformconfig "openclaw/platform/internal/config"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/upstream"
)

type State struct {
	OfficialRoutes []upstream.OfficialRoute    `json:"official_routes"`
	OfficialModels []service.OfficialModel     `json:"official_models"`
	PricingRules   []service.PricingRule       `json:"pricing_rules"`
	Agreements     []service.AgreementDocument `json:"agreements"`
}

type Manager struct {
	mu      sync.RWMutex
	path    string
	service *service.Service
	router  *upstream.Router
	state   State
}

func NewManager(path string, svc *service.Service, router *upstream.Router) *Manager {
	return &Manager{
		path:    strings.TrimSpace(path),
		service: svc,
		router:  router,
	}
}

func BuildStateFromEnv(cfg platformconfig.Config) (State, error) {
	var state State
	if err := decodeOptionalJSON(cfg.OfficialRoutesJSON, &state.OfficialRoutes); err != nil {
		return State{}, fmt.Errorf("official routes json: %w", err)
	}
	if err := decodeOptionalJSON(cfg.OfficialModelsJSON, &state.OfficialModels); err != nil {
		return State{}, fmt.Errorf("official models json: %w", err)
	}
	if err := decodeOptionalJSON(cfg.PricingRulesJSON, &state.PricingRules); err != nil {
		return State{}, fmt.Errorf("pricing rules json: %w", err)
	}
	if err := decodeOptionalJSON(cfg.AgreementsJSON, &state.Agreements); err != nil {
		return State{}, fmt.Errorf("agreements json: %w", err)
	}
	return normalizeState(state)
}

func (m *Manager) Bootstrap(seed State) error {
	var (
		state      State
		seedToFile bool
	)
	if m.path != "" {
		loaded, err := loadStateFile(m.path)
		switch {
		case err == nil:
			state = loaded
		case errors.Is(err, os.ErrNotExist):
			state = seed
			seedToFile = true
		default:
			return err
		}
	} else {
		state = seed
	}
	if err := m.applyState(state); err != nil {
		return err
	}
	if seedToFile {
		return m.writeStateFile(m.Snapshot())
	}
	return nil
}

func (m *Manager) Snapshot() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneState(m.state)
}

func (m *Manager) Save(state State) error {
	normalized, err := normalizeState(state)
	if err != nil {
		return err
	}
	if m.path != "" {
		if err := m.writeStateFile(normalized); err != nil {
			return err
		}
	}
	if err := m.applyNormalizedState(normalized); err != nil {
		return err
	}
	return nil
}

func (m *Manager) applyState(state State) error {
	normalized, err := normalizeState(state)
	if err != nil {
		return err
	}
	return m.applyNormalizedState(normalized)
}

func (m *Manager) applyNormalizedState(normalized State) error {
	if m.router != nil {
		m.router.UpdateRoutes(normalized.OfficialRoutes)
	}
	if m.service != nil {
		m.service.SetOfficialModels(normalized.OfficialModels)
		m.service.SetPricingCatalog(normalized.PricingRules)
		m.service.SetAgreements(normalized.Agreements)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = normalized
	return nil
}

func (m *Manager) writeStateFile(state State) error {
	if strings.TrimSpace(m.path) == "" {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o600)
}

func loadStateFile(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode runtime config: %w", err)
	}
	return normalizeState(state)
}

func normalizeState(state State) (State, error) {
	routes := normalizeRoutes(state.OfficialRoutes)
	for _, route := range routes {
		if err := route.ModelConfig.Validate(); err != nil {
			return State{}, fmt.Errorf("official route %q: %w", route.PublicModelID, err)
		}
	}
	models := normalizeModels(state.OfficialModels, routes)
	pricing := normalizePricingRules(state.PricingRules)
	agreements := normalizeAgreements(state.Agreements)

	routeIDs := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		routeIDs[route.PublicModelID] = struct{}{}
	}
	for _, model := range models {
		if !model.Enabled {
			continue
		}
		if _, ok := routeIDs[model.ID]; !ok {
			return State{}, fmt.Errorf("enabled model %q is missing an official route", model.ID)
		}
	}

	return State{
		OfficialRoutes: routes,
		OfficialModels: models,
		PricingRules:   pricing,
		Agreements:     agreements,
	}, nil
}

func normalizeRoutes(routes []upstream.OfficialRoute) []upstream.OfficialRoute {
	seen := make(map[string]upstream.OfficialRoute, len(routes))
	for _, route := range routes {
		id := strings.TrimSpace(route.PublicModelID)
		if id == "" {
			continue
		}
		cfg := route.ModelConfig
		cfg.ModelName = strings.TrimSpace(cfg.ModelName)
		if cfg.ModelName == "" {
			cfg.ModelName = id
		}
		cfg.Model = strings.TrimSpace(cfg.Model)
		cfg.APIBase = strings.TrimSpace(cfg.APIBase)
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		cfg.Proxy = strings.TrimSpace(cfg.Proxy)
		cfg.AuthMethod = strings.TrimSpace(cfg.AuthMethod)
		cfg.ConnectMode = strings.TrimSpace(cfg.ConnectMode)
		cfg.Workspace = strings.TrimSpace(cfg.Workspace)
		seen[id] = upstream.OfficialRoute{
			PublicModelID: id,
			ModelConfig:   cfg,
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]upstream.OfficialRoute, 0, len(keys))
	for _, key := range keys {
		items = append(items, seen[key])
	}
	return items
}

func normalizeModels(models []service.OfficialModel, routes []upstream.OfficialRoute) []service.OfficialModel {
	if len(models) == 0 {
		models = make([]service.OfficialModel, 0, len(routes))
		for _, route := range routes {
			models = append(models, service.OfficialModel{
				ID:             route.PublicModelID,
				Name:           route.PublicModelID,
				Description:    "",
				Enabled:        true,
				PricingVersion: "",
			})
		}
	}

	seen := make(map[string]service.OfficialModel, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = id
		}
		description := strings.TrimSpace(model.Description)
		pricingVersion := strings.TrimSpace(model.PricingVersion)
		seen[id] = service.OfficialModel{
			ID:             id,
			Name:           name,
			Description:    description,
			Enabled:        model.Enabled,
			PricingVersion: pricingVersion,
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]service.OfficialModel, 0, len(keys))
	for _, key := range keys {
		items = append(items, seen[key])
	}
	return items
}

func normalizePricingRules(rules []service.PricingRule) []service.PricingRule {
	seen := make(map[string]service.PricingRule, len(rules))
	for _, rule := range rules {
		modelID := strings.TrimSpace(rule.ModelID)
		if modelID == "" {
			continue
		}
		rule.ModelID = modelID
		rule.Version = strings.TrimSpace(rule.Version)
		if rule.Version == "" {
			rule.Version = "v1"
		}
		seen[modelID+"::"+rule.Version] = rule
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]service.PricingRule, 0, len(keys))
	for _, key := range keys {
		items = append(items, seen[key])
	}
	return items
}

func normalizeAgreements(docs []service.AgreementDocument) []service.AgreementDocument {
	seen := make(map[string]service.AgreementDocument, len(docs))
	for _, doc := range docs {
		key := strings.TrimSpace(doc.Key)
		version := strings.TrimSpace(doc.Version)
		if key == "" || version == "" {
			continue
		}
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = key
		}
		content := strings.TrimSpace(doc.Content)
		linkURL := strings.TrimSpace(doc.URL)
		seen[key+"::"+version] = service.AgreementDocument{
			Key:               key,
			Version:           version,
			Title:             title,
			Content:           content,
			URL:               linkURL,
			EffectiveFromUnix: doc.EffectiveFromUnix,
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]service.AgreementDocument, 0, len(keys))
	for _, key := range keys {
		items = append(items, seen[key])
	}
	return items
}

func cloneState(state State) State {
	return State{
		OfficialRoutes: append([]upstream.OfficialRoute(nil), state.OfficialRoutes...),
		OfficialModels: append([]service.OfficialModel(nil), state.OfficialModels...),
		PricingRules:   append([]service.PricingRule(nil), state.PricingRules...),
		Agreements:     append([]service.AgreementDocument(nil), state.Agreements...),
	}
}

func decodeOptionalJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}
