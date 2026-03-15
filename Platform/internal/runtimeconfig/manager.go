package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	platformconfig "openclaw/platform/internal/config"
	"openclaw/platform/internal/revisiontoken"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/upstream"
)

type State struct {
	OfficialRoutes []upstream.OfficialRoute    `json:"official_routes"`
	OfficialModels []service.OfficialModel     `json:"official_models"`
	PricingRules   []service.PricingRule       `json:"pricing_rules"`
	Agreements     []service.AgreementDocument `json:"agreements"`
}

const RedactedSecretPlaceholder = "__KEEP_EXISTING_SECRET__"

var allowedOfficialRouteProtocols = map[string]struct{}{
	"anthropic":        {},
	"avian":            {},
	"cerebras":         {},
	"deepseek":         {},
	"gemini":           {},
	"groq":             {},
	"litellm":          {},
	"mistral":          {},
	"moonshot":         {},
	"nvidia":           {},
	"official":         {},
	"openai":           {},
	"openai-responses": {},
	"openrouter":       {},
	"qwen":             {},
	"responses":        {},
	"shengsuanyun":     {},
	"vllm":             {},
	"vivgrid":          {},
	"volcengine":       {},
	"zhipu":            {},
}

var lookupOfficialRouteHostIPs = func(host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIP(context.Background(), "ip", host)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}

type Manager struct {
	mu      sync.RWMutex
	writeMu sync.Mutex
	path    string
	service *service.Service
	router  *upstream.Router
	state   State
}

const (
	runtimeConfigLockTimeout  = 5 * time.Second
	runtimeConfigStaleLockAge = 30 * time.Second
)

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

func (m *Manager) RedactedSnapshot() State {
	return RedactState(m.Snapshot())
}

func (m *Manager) Save(state State) error {
	return m.SaveWithRevision("", state)
}

func (m *Manager) SaveWithRevision(expectedRevision string, state State) error {
	return m.mutateWithRevision(expectedRevision, func(current State) (any, State) {
		return revisionComparableState(current), RestoreRedactedSecrets(current, state)
	})
}

func (m *Manager) SaveModelsWithRevision(expectedRevision string, models []service.OfficialModel) error {
	return m.mutateWithRevision(expectedRevision, func(current State) (any, State) {
		next := cloneState(current)
		next.OfficialModels = append([]service.OfficialModel(nil), models...)
		return current.OfficialModels, next
	})
}

func (m *Manager) SaveRoutesWithRevision(expectedRevision string, routes []upstream.OfficialRoute) error {
	return m.mutateWithRevision(expectedRevision, func(current State) (any, State) {
		next := cloneState(current)
		next.OfficialRoutes = mergeOfficialRouteSecrets(current.OfficialRoutes, routes)
		return revisionComparableRoutes(current.OfficialRoutes), next
	})
}

func (m *Manager) SavePricingRulesWithRevision(expectedRevision string, rules []service.PricingRule) error {
	return m.mutateWithRevision(expectedRevision, func(current State) (any, State) {
		next := cloneState(current)
		next.PricingRules = append([]service.PricingRule(nil), rules...)
		return current.PricingRules, next
	})
}

func (m *Manager) SaveAgreementsWithRevision(expectedRevision string, docs []service.AgreementDocument) error {
	return m.mutateWithRevision(expectedRevision, func(current State) (any, State) {
		next := cloneState(current)
		next.Agreements = append([]service.AgreementDocument(nil), docs...)
		return current.Agreements, next
	})
}

func (m *Manager) mutateWithRevision(expectedRevision string, mutate func(current State) (currentPayload any, next State)) error {
	expectedRevision = strings.TrimSpace(expectedRevision)
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	release, err := m.acquireFileLock()
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}

	current, err := m.loadCurrentStateForWrite()
	if err != nil {
		return err
	}
	currentPayload, next := mutate(current)
	if expectedRevision != "" {
		revision, err := revisiontoken.ForPayload(currentPayload)
		if err != nil {
			return err
		}
		if !revisiontoken.Matches(expectedRevision, revision) {
			return service.ErrRevisionConflict
		}
	}

	normalized, err := normalizeState(next)
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

func (m *Manager) loadCurrentStateForWrite() (State, error) {
	if strings.TrimSpace(m.path) != "" {
		loaded, err := loadStateFile(m.path)
		switch {
		case err == nil:
			return loaded, nil
		case errors.Is(err, os.ErrNotExist):
			return m.Snapshot(), nil
		default:
			return State{}, err
		}
	}
	return m.Snapshot(), nil
}

func (m *Manager) acquireFileLock() (func(), error) {
	if strings.TrimSpace(m.path) == "" {
		return nil, nil
	}
	lockPath := m.path + ".lock"
	deadline := time.Now().Add(runtimeConfigLockTimeout)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = file.WriteString(fmt.Sprintf("%d", time.Now().UnixNano()))
			_ = file.Close()
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > runtimeConfigStaleLockAge {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("runtime config lock timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
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
		if err := validateOfficialRoute(route); err != nil {
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

func validateOfficialRoute(route upstream.OfficialRoute) error {
	protocol, _, found := strings.Cut(strings.TrimSpace(route.ModelConfig.Model), "/")
	if !found {
		protocol = "openai"
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if _, ok := allowedOfficialRouteProtocols[protocol]; !ok {
		return fmt.Errorf("protocol %q is not allowed for official routes", protocol)
	}
	if requiresExplicitOfficialRouteAPIBase(protocol) && strings.TrimSpace(route.ModelConfig.APIBase) == "" {
		return fmt.Errorf("protocol %q requires a non-loopback api_base for official routes", protocol)
	}
	for _, endpoint := range []struct {
		name  string
		value string
	}{
		{name: "api_base", value: route.ModelConfig.APIBase},
		{name: "proxy", value: route.ModelConfig.Proxy},
	} {
		if err := validateOfficialRouteEndpoint(endpoint.name, endpoint.value); err != nil {
			return err
		}
	}
	return nil
}

func requiresExplicitOfficialRouteAPIBase(protocol string) bool {
	switch protocol {
	case "official", "litellm", "vllm":
		return true
	default:
		return false
	}
}

func validateOfficialRouteEndpoint(name, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("%s host is required", name)
	}
	if isPrivateOfficialRouteHost(host) {
		return fmt.Errorf("%s host %q is not allowed for official routes", name, host)
	}
	if resolved, err := lookupOfficialRouteHostIPs(host); err == nil {
		for _, ip := range resolved {
			if isPrivateOfficialRouteHost(ip.String()) {
				return fmt.Errorf("%s host %q resolves to a private address", name, host)
			}
		}
	}
	return nil
}

func isPrivateOfficialRouteHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "localhost", "0.0.0.0", "::1":
		return true
	}
	if strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func RedactState(state State) State {
	out := cloneState(state)
	out.OfficialRoutes = redactOfficialRoutes(out.OfficialRoutes)
	return out
}

func RestoreRedactedSecrets(current State, incoming State) State {
	out := cloneState(incoming)
	out.OfficialRoutes = mergeOfficialRouteSecrets(current.OfficialRoutes, incoming.OfficialRoutes)
	return out
}

func redactOfficialRoutes(routes []upstream.OfficialRoute) []upstream.OfficialRoute {
	items := append([]upstream.OfficialRoute(nil), routes...)
	for i := range items {
		if strings.TrimSpace(items[i].ModelConfig.APIKey) != "" {
			items[i].ModelConfig.APIKey = RedactedSecretPlaceholder
		}
		items[i].ModelConfig.APIBase = redactEndpointURL(items[i].ModelConfig.APIBase)
		items[i].ModelConfig.Proxy = redactEndpointURL(items[i].ModelConfig.Proxy)
	}
	return items
}

func redactEndpointURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	return parsed.String()
}

func mergeOfficialRouteSecrets(current, incoming []upstream.OfficialRoute) []upstream.OfficialRoute {
	type endpointSecrets struct {
		apiKey  string
		apiBase string
		proxy   string
	}
	existingSecrets := make(map[string]endpointSecrets, len(current))
	for _, route := range current {
		existingSecrets[strings.TrimSpace(route.PublicModelID)] = endpointSecrets{
			apiKey:  route.ModelConfig.APIKey,
			apiBase: route.ModelConfig.APIBase,
			proxy:   route.ModelConfig.Proxy,
		}
	}
	items := append([]upstream.OfficialRoute(nil), incoming...)
	for i := range items {
		existing := existingSecrets[strings.TrimSpace(items[i].PublicModelID)]
		if strings.TrimSpace(items[i].ModelConfig.APIKey) == RedactedSecretPlaceholder {
			items[i].ModelConfig.APIKey = existing.apiKey
		}
		if redactEndpointURL(items[i].ModelConfig.APIBase) == redactEndpointURL(existing.apiBase) {
			items[i].ModelConfig.APIBase = existing.apiBase
		}
		if redactEndpointURL(items[i].ModelConfig.Proxy) == redactEndpointURL(existing.proxy) {
			items[i].ModelConfig.Proxy = existing.proxy
		}
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
