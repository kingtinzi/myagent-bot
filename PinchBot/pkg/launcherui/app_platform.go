package launcherui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func RegisterAppPlatformAPI(mux *http.ServeMux, absPath string) {
	mux.HandleFunc("GET /api/app/auth/agreements", func(w http.ResponseWriter, r *http.Request) {
		client, err := platformClientForConfig(absPath)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		docs, err := client.ListAgreements(r.Context(), "")
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(platformapi.FilterAuthAgreements(docs))
	})

	mux.HandleFunc("GET /api/app/session", func(w http.ResponseWriter, r *http.Request) {
		store := sessionStoreForConfig(absPath)
		session, err := store.Load()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
			return
		}
		if session.IsExpired(time.Now()) {
			_ = store.Clear()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
			return
		}
		client, err := platformClientForConfig(absPath)
		if err != nil {
			http.Error(w, platformUserFacingError(absPath, err), http.StatusBadGateway)
			return
		}
		wallet, walletErr := client.GetWallet(r.Context(), session.AccessToken)
		resp := map[string]any{
			"authenticated": true,
			"session":       session.View(),
			"wallet":        wallet,
		}
		if walletErr != nil {
			if platformapi.IsStatusCode(walletErr, http.StatusUnauthorized) {
				_ = store.Clear()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
				return
			}
			resp["wallet_error"] = platformUserFacingError(absPath, walletErr)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /api/app/auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleAppAuthMutation(w, r, absPath, true)
	})
	mux.HandleFunc("POST /api/app/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		handleAppAuthMutation(w, r, absPath, false)
	})
	mux.HandleFunc("POST /api/app/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := sessionStoreForConfig(absPath).Clear(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/app/wallet", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		wallet, err := client.GetWallet(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wallet)
	})

	mux.HandleFunc("GET /api/app/models", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		models, err := client.ListOfficialModels(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildAppOfficialModelSummaries(models))
	})

	mux.HandleFunc("GET /api/app/official-access", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		state, err := client.GetOfficialAccessState(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	})

	mux.HandleFunc("POST /api/app/models/sync", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		models, err := client.ListOfficialModels(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		result, err := syncOfficialModelsIntoConfig(absPath, models)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/app/agreements", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		docs, err := client.ListAgreements(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(docs)
	})

	mux.HandleFunc("POST /api/app/agreements/accept", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		var req platformapi.AcceptAgreementsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := client.AcceptAgreements(r.Context(), session.AccessToken, req); err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		session.AgreementSyncPending = false
		session.Warning = ""
		if err := sessionStoreForConfig(absPath).Save(session); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/app/transactions", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		items, err := client.ListTransactions(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	mux.HandleFunc("GET /api/app/refund-requests", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		items, err := client.ListRefundRequests(r.Context(), session.AccessToken)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	mux.HandleFunc("POST /api/app/orders", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		var req platformapi.CreateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		order, err := client.CreateOrder(r.Context(), session.AccessToken, req)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(order)
	})

	mux.HandleFunc("GET /api/app/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		order, err := client.GetOrder(r.Context(), session.AccessToken, r.PathValue("id"))
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(order)
	})

	mux.HandleFunc("POST /api/app/refund-requests", func(w http.ResponseWriter, r *http.Request) {
		client, session, ok := platformContext(w, r, absPath)
		if !ok {
			return
		}
		var req platformapi.CreateRefundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		item, err := client.CreateRefundRequest(r.Context(), session.AccessToken, req)
		if err != nil {
			writePlatformAPIError(w, absPath, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(item)
	})

	mux.HandleFunc("GET /api/app/backend-status", func(w http.ResponseWriter, r *http.Request) {
		client, err := platformClientForConfig(absPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		cfg, err := config.LoadConfig(absPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		gatewayURL := gatewayBaseURLFromConfig(cfg)
		platformURL := clientBaseURL(client)
		settingsURL := requestBaseURL(r)
		resp := platformapi.BackendStatus{
			GatewayURL:      gatewayURL,
			GatewayHealthy:  probeHealth(gatewayURL + "/health"),
			PlatformURL:     platformURL,
			PlatformHealthy: probeHealth(platformURL + "/health"),
			SettingsURL:     settingsURL,
			SettingsHealthy: probeHealth(settingsURL + "/api/config"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

type appOfficialModelSummary struct {
	Enabled        bool   `json:"enabled"`
	PricingSummary string `json:"pricing_summary,omitempty"`
	PricingVersion string `json:"pricing_version,omitempty"`
}

func buildAppOfficialModelSummaries(models []platformapi.OfficialModel) []appOfficialModelSummary {
	if len(models) == 0 {
		return []appOfficialModelSummary{}
	}
	items := make([]appOfficialModelSummary, 0, len(models))
	for _, model := range models {
		items = append(items, appOfficialModelSummary{
			Enabled:        model.Enabled,
			PricingSummary: appOfficialPricingSummary(model),
			PricingVersion: strings.TrimSpace(model.PricingVersion),
		})
	}
	return items
}

func appOfficialPricingSummary(model platformapi.OfficialModel) string {
	raw := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(model.ID),
		strings.TrimSpace(model.Name),
		strings.TrimSpace(model.PricingVersion),
	}, " "))
	if strings.Contains(raw, "gpt-5.2") || strings.Contains(raw, "gpt-5-2") {
		return "3 元人民币 / 100 万 Token"
	}
	return ""
}

func handleAppAuthMutation(w http.ResponseWriter, r *http.Request, absPath string, login bool) {
	var req struct {
		Email      string                          `json:"email"`
		Password   string                          `json:"password"`
		Username   string                          `json:"username,omitempty"`
		Agreements []platformapi.AgreementDocument `json:"agreements,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("invalid json"), http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !platformapi.IsLikelyValidEmailAddress(req.Email) {
		http.Error(w, platformapi.InvalidEmailFormatMessage, http.StatusBadRequest)
		return
	}
	client, err := platformClientForConfig(absPath)
	if err != nil {
		writePlatformAPIError(w, absPath, err)
		return
	}
	var (
		authResp platformapi.AuthResponse
		session  platformapi.Session
	)
	if login {
		authResp, err = client.LoginResponse(r.Context(), platformapi.AuthRequest{Email: req.Email, Password: req.Password})
	} else {
		authResp, err = client.SignUpResponse(r.Context(), platformapi.AuthRequest{
			Email:      req.Email,
			Password:   req.Password,
			Username:   strings.TrimSpace(req.Username),
			Agreements: platformapi.FilterAuthAgreements(req.Agreements),
		})
	}
	if err != nil {
		writePlatformAPIError(w, absPath, err)
		return
	}
	session = authResp.Session
	session.AccessToken = strings.TrimSpace(session.AccessToken)
	if session.AccessToken == "" {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("authentication service did not return a valid session"), http.StatusBadGateway)
		return
	}
	warning := platformapi.NormalizeUserFacingErrorMessage(authResp.Warning)
	if !login && len(req.Agreements) > 0 && authResp.AgreementSyncRequired {
		if err := client.AcceptAgreements(r.Context(), session.AccessToken, platformapi.AcceptAgreementsRequest{
			Agreements: platformapi.FilterAuthAgreements(req.Agreements),
		}); err != nil {
			warning = platformapi.NormalizeUserFacingErrorMessage("signup succeeded, but agreement sync must be retried before recharge")
			session.AgreementSyncPending = true
			session.Warning = warning
		} else {
			warning = ""
			session.AgreementSyncPending = false
			session.Warning = ""
		}
	}
	store := sessionStoreForConfig(absPath)
	if err := store.Save(session); err != nil {
		http.Error(w, "登录状态保存失败，请稍后重试", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(platformapi.BrowserAuthResponse{
		Session: session.View(),
		Warning: warning,
	})
}

func platformClientForConfig(absPath string) (*platformapi.Client, error) {
	cfg, err := config.LoadConfig(absPath)
	if err != nil {
		return nil, err
	}
	return platformapi.NewClient(cfg.PlatformAPI.BaseURL), nil
}

func sessionStoreForConfig(absPath string) *platformapi.FileSessionStore {
	return platformapi.NewFileSessionStore(config.GetPinchBotHome())
}

func platformContext(
	w http.ResponseWriter,
	r *http.Request,
	absPath string,
) (*platformapi.Client, platformapi.Session, bool) {
	session, err := sessionStoreForConfig(absPath).Load()
	if err != nil {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("not logged in"), http.StatusUnauthorized)
		return nil, platformapi.Session{}, false
	}
	if session.IsExpired(time.Now()) {
		_ = sessionStoreForConfig(absPath).Clear()
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("not logged in"), http.StatusUnauthorized)
		return nil, platformapi.Session{}, false
	}
	client, err := platformClientForConfig(absPath)
	if err != nil {
		http.Error(w, platformUserFacingError(absPath, err), http.StatusBadGateway)
		return nil, platformapi.Session{}, false
	}
	return client, session, true
}

func writePlatformAPIError(w http.ResponseWriter, absPath string, err error) {
	status := http.StatusBadGateway
	if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
		_ = sessionStoreForConfig(absPath).Clear()
		status = http.StatusUnauthorized
	}
	var apiErr *platformapi.APIError
	if status == http.StatusBadGateway && errors.As(err, &apiErr) && apiErr.StatusCode > 0 {
		status = apiErr.StatusCode
	}
	http.Error(w, platformUserFacingError(absPath, err), status)
}

func platformUserFacingError(absPath string, err error) string {
	if err == nil {
		return ""
	}
	if msg := platformapi.UserFacingErrorMessage(err); msg != "" && msg != strings.TrimSpace(err.Error()) {
		return msg
	}
	var apiErr *platformapi.APIError
	if errors.As(err, &apiErr) {
		return platformapi.UserFacingErrorMessage(err)
	}
	if client, clientErr := platformClientForConfig(absPath); clientErr == nil {
		if isLocalPlatformBaseURL(client.BaseURL()) && isLikelyConnectionRefusedError(err) {
			return "本地平台注册服务不可用，请检查 platform-server 是否已启动"
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "config") {
		return "平台配置加载失败，请检查设置后重试"
	}
	return "平台服务暂不可用，请稍后重试"
}

func isLocalPlatformBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return host == "127.0.0.1" || host == "localhost" || host == ""
}

func isLikelyConnectionRefusedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"connection refused",
		"actively refused",
		"connectex",
		"dial tcp",
		"no connection could be made",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	var netErr *net.OpError
	return errors.As(err, &netErr)
}

type officialModelSyncResult struct {
	Added          int    `json:"added"`
	Updated        int    `json:"updated"`
	Removed        int    `json:"removed"`
	Total          int    `json:"total"`
	DefaultModel   string `json:"default_model,omitempty"`
	DefaultChanged bool   `json:"default_changed,omitempty"`
	Warning        string `json:"warning,omitempty"`
}

const (
	canonicalOfficialModelAlias = "official"
	canonicalOfficialModelRef   = "official/default"
)

func syncOfficialModelsIntoConfig(absPath string, models []platformapi.OfficialModel) (officialModelSyncResult, error) {
	cfg, err := config.LoadConfig(absPath)
	if err != nil {
		return officialModelSyncResult{}, err
	}
	baseURL := strings.TrimSpace(cfg.PlatformAPI.BaseURL)
	if baseURL == "" {
		baseURL = config.DefaultConfig().PlatformAPI.BaseURL
	}

	enabledOrder := collectEnabledOfficialModelIDs(models)
	needsCanonicalOfficialAlias := len(enabledOrder) > 0 || configContainsOfficialModel(cfg.ModelList)

	result := officialModelSyncResult{}
	defaultModel := cfg.Agents.Defaults.GetModelName()
	originalDefaultModel := defaultModel
	defaultRemoved := false
	out := make([]config.ModelConfig, 0, len(cfg.ModelList)+1)
	existingOfficial := make([]config.ModelConfig, 0, len(cfg.ModelList))
	usedModelNames := make(map[string]struct{}, len(cfg.ModelList)+1)
	if needsCanonicalOfficialAlias {
		rememberModelAlias(usedModelNames, canonicalOfficialModelAlias)
	}
	// Reserve all non-official aliases up front so conflict renames never collide
	// with aliases that appear later in the list.
	for _, item := range cfg.ModelList {
		if _, isOfficial := officialModelID(item.Model); isOfficial {
			continue
		}
		if isBootstrapSampleModel(item) {
			continue
		}
		rememberModelAlias(usedModelNames, item.ModelName)
	}

	for _, item := range cfg.ModelList {
		if _, isOfficial := officialModelID(item.Model); isOfficial {
			existingOfficial = append(existingOfficial, item)
			if item.ModelName == defaultModel {
				defaultRemoved = true
			}
			continue
		}
		if isBootstrapSampleModel(item) {
			result.Removed++
			if item.ModelName == defaultModel {
				defaultRemoved = true
			}
			continue
		}
		if needsCanonicalOfficialAlias && strings.EqualFold(strings.TrimSpace(item.ModelName), canonicalOfficialModelAlias) {
			originalModelName := item.ModelName
			item.ModelName = nextAvailableModelAlias("official-custom", usedModelNames)
			result.Updated++
			if !defaultRemoved && strings.EqualFold(strings.TrimSpace(defaultModel), strings.TrimSpace(originalModelName)) {
				defaultModel = item.ModelName
			}
		}
		out = append(out, item)
	}

	imported := make([]string, 0, 1)
	if len(enabledOrder) == 0 {
		if len(existingOfficial) > 0 {
			preserved := existingOfficial[0]
			preserved.ModelName = canonicalOfficialModelAlias
			preserved.APIBase = baseURL
			preserved.APIKey = ""
			preserved.Proxy = ""
			preserved.Fallbacks = nil
			out = append(out, preserved)
			imported = append(imported, canonicalOfficialModelAlias)
			result.Warning = "当前未返回可用官方模型，已保留本地官方模型配置。"
			if !reflect.DeepEqual(existingOfficial[0], preserved) {
				result.Updated++
			}
			if len(existingOfficial) > 1 {
				result.Removed += len(existingOfficial) - 1
			}
		} else {
			result.Warning = "当前未返回可用官方模型，请稍后重试或联系管理员检查官方模型配置。"
		}
	} else {
		canonical := buildUnifiedOfficialModel(existingOfficial, baseURL)
		out = append(out, canonical)
		imported = append(imported, canonicalOfficialModelAlias)

		if len(existingOfficial) == 0 {
			result.Added++
		} else {
			result.Updated++
			if len(existingOfficial) > 1 {
				result.Removed += len(existingOfficial) - 1
			}
		}
	}

	cfg.ModelList = out
	cfg.Agents.Defaults.ModelName = defaultModel
	result.Total = len(out)
	if shouldPromoteOfficialDefault(cfg, defaultModel, defaultRemoved, imported) {
		if len(imported) > 0 {
			cfg.Agents.Defaults.ModelName = imported[0]
		} else if len(out) > 0 {
			cfg.Agents.Defaults.ModelName = out[0].ModelName
		} else {
			cfg.Agents.Defaults.ModelName = ""
		}
	}
	if cfg.Agents.Defaults.ModelName != originalDefaultModel {
		result.DefaultChanged = true
		result.DefaultModel = cfg.Agents.Defaults.ModelName
	}
	if err := config.SaveConfig(absPath, cfg); err != nil {
		return officialModelSyncResult{}, err
	}
	return result, nil
}

func collectEnabledOfficialModelIDs(models []platformapi.OfficialModel) []string {
	type candidate struct {
		id       string
		priority int
		index    int
	}

	collected := make([]candidate, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" || !model.Enabled {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		priority := model.FallbackPriority
		if priority < 0 {
			priority = 0
		}
		collected = append(collected, candidate{
			id:       id,
			priority: priority,
			index:    len(collected),
		})
	}

	sort.Slice(collected, func(i, j int) bool {
		if collected[i].priority != collected[j].priority {
			return collected[i].priority < collected[j].priority
		}
		return collected[i].index < collected[j].index
	})

	out := make([]string, 0, len(collected))
	for _, item := range collected {
		out = append(out, item.id)
	}
	return out
}

func buildUnifiedOfficialModel(existing []config.ModelConfig, baseURL string) config.ModelConfig {
	model := config.ModelConfig{}
	if len(existing) > 0 {
		model = existing[0]
	}
	model.ModelName = canonicalOfficialModelAlias
	model.Model = canonicalOfficialModelRef
	model.APIBase = strings.TrimSpace(baseURL)
	model.APIKey = ""
	model.Proxy = ""
	model.Fallbacks = nil
	return model
}

func configContainsOfficialModel(models []config.ModelConfig) bool {
	for _, item := range models {
		if _, isOfficial := officialModelID(item.Model); isOfficial {
			return true
		}
	}
	return false
}

func rememberModelAlias(used map[string]struct{}, alias string) {
	key := strings.ToLower(strings.TrimSpace(alias))
	if key == "" {
		return
	}
	used[key] = struct{}{}
}

func nextAvailableModelAlias(base string, used map[string]struct{}) string {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		base = "model"
	}
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func officialModelID(model string) (string, bool) {
	protocol, modelID, found := strings.Cut(strings.TrimSpace(model), "/")
	if !found {
		return "", false
	}
	if protocol != "official" {
		return "", false
	}
	modelID = strings.TrimSpace(modelID)
	return modelID, modelID != ""
}

func shouldPromoteOfficialDefault(cfg *config.Config, defaultModel string, defaultRemoved bool, imported []string) bool {
	if defaultRemoved || strings.TrimSpace(defaultModel) == "" {
		return true
	}
	if len(imported) == 0 || cfg == nil {
		return false
	}
	current, err := cfg.GetModelConfig(defaultModel)
	if err != nil || current == nil {
		return true
	}
	if _, isOfficial := officialModelID(current.Model); isOfficial {
		return false
	}
	return isBootstrapSampleModel(*current)
}

func isBootstrapSampleModel(item config.ModelConfig) bool {
	if strings.TrimSpace(item.AuthMethod) != "" {
		return false
	}
	apiKey := strings.TrimSpace(item.APIKey)
	if apiKey != "" && looksLikePlaceholderSecret(apiKey) {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(item.Model))
	apiBase := strings.ToLower(strings.TrimSpace(item.APIBase))
	switch model {
	case "openai/gpt-5.2":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.openai.com/v1")
	case "anthropic/claude-sonnet-4.6":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.anthropic.com/v1")
	case "deepseek/deepseek-chat":
		return apiKey == "" && (apiBase == "" || apiBase == "https://api.deepseek.com/v1")
	case "qwen/qwen-plus":
		return apiKey == "" && (apiBase == "" || apiBase == "https://dashscope.aliyuncs.com/compatible-mode/v1")
	default:
		return false
	}
}

func looksLikePlaceholderSecret(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return true
	}
	placeholders := []string{
		"sk-your-openai-key",
		"sk-ant-your-key",
		"your_dashscope_key",
		"your-dashscope-key",
		"replace-with-your-upstream-api-key",
		"your_api_key",
		"your-api-key",
		"gsk_xxx",
		"sk-xxx",
	}
	for _, placeholder := range placeholders {
		if value == placeholder {
			return true
		}
	}
	return strings.Contains(value, "your-key") || strings.Contains(value, "your_api_key")
}

func clientBaseURL(client *platformapi.Client) string {
	if client == nil {
		return ""
	}
	return client.BaseURL()
}

func gatewayBaseURLFromConfig(cfg *config.Config) string {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	host := strings.TrimSpace(cfg.Gateway.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Gateway.Port
	if port <= 0 {
		port = 18790
	}
	return "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Host)
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}
	if host == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + host
}

func probeHealth(url string) bool {
	if strings.TrimSpace(url) == "" {
		return false
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
