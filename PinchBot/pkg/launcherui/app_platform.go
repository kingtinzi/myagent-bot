package launcherui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
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
		json.NewEncoder(w).Encode(models)
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

func syncOfficialModelsIntoConfig(absPath string, models []platformapi.OfficialModel) (officialModelSyncResult, error) {
	cfg, err := config.LoadConfig(absPath)
	if err != nil {
		return officialModelSyncResult{}, err
	}
	baseURL := strings.TrimSpace(cfg.PlatformAPI.BaseURL)
	if baseURL == "" {
		baseURL = config.DefaultConfig().PlatformAPI.BaseURL
	}

	enabled := make(map[string]platformapi.OfficialModel, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || !model.Enabled {
			continue
		}
		enabled[model.ID] = model
	}

	result := officialModelSyncResult{}
	defaultModel := cfg.Agents.Defaults.GetModelName()
	defaultRemoved := false
	out := make([]config.ModelConfig, 0, len(cfg.ModelList)+len(enabled))
	seen := make(map[string]struct{}, len(enabled))
	preserveExistingOfficialModels := len(enabled) == 0

	for _, item := range cfg.ModelList {
		modelID, isOfficial := officialModelID(item.Model)
		if !isOfficial {
			if isBootstrapSampleModel(item) {
				result.Removed++
				if item.ModelName == defaultModel {
					defaultRemoved = true
				}
				continue
			}
			out = append(out, item)
			continue
		}
		if preserveExistingOfficialModels {
			out = append(out, item)
			continue
		}
		model, ok := enabled[modelID]
		if !ok {
			result.Removed++
			if item.ModelName == defaultModel {
				defaultRemoved = true
			}
			continue
		}
		alias := officialModelAlias(model)
		updated := item
		if strings.TrimSpace(updated.ModelName) == "" || strings.HasPrefix(strings.TrimSpace(updated.ModelName), "official-") {
			updated.ModelName = alias
		}
		updated.Model = "official/" + model.ID
		updated.APIBase = baseURL
		updated.APIKey = ""
		updated.Proxy = ""
		if !reflect.DeepEqual(item, updated) {
			result.Updated++
		}
		out = append(out, updated)
		seen[model.ID] = struct{}{}
	}

	imported := make([]string, 0, len(enabled))
	if preserveExistingOfficialModels {
		for _, existing := range out {
			if _, isOfficial := officialModelID(existing.Model); isOfficial {
				imported = append(imported, existing.ModelName)
			}
		}
		if len(imported) > 0 {
			result.Warning = "当前未返回可用官方模型，已保留本地官方模型配置。"
		} else {
			result.Warning = "当前未返回可用官方模型，请稍后重试或联系管理员检查官方模型配置。"
		}
	} else {
		for _, model := range models {
			model.ID = strings.TrimSpace(model.ID)
			if model.ID == "" || !model.Enabled {
				continue
			}
			if _, ok := seen[model.ID]; ok {
				for _, existing := range out {
					if existing.Model == "official/"+model.ID {
						imported = append(imported, existing.ModelName)
						break
					}
				}
				continue
			}
			out = append(out, config.ModelConfig{
				ModelName: officialModelAlias(model),
				Model:     "official/" + model.ID,
				APIBase:   baseURL,
			})
			seen[model.ID] = struct{}{}
			imported = append(imported, officialModelAlias(model))
			result.Added++
		}
	}

	cfg.ModelList = out
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
	if cfg.Agents.Defaults.ModelName != defaultModel {
		result.DefaultChanged = true
		result.DefaultModel = cfg.Agents.Defaults.ModelName
	}
	if err := config.SaveConfig(absPath, cfg); err != nil {
		return officialModelSyncResult{}, err
	}
	return result, nil
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

func officialModelAlias(model platformapi.OfficialModel) string {
	label := strings.TrimSpace(model.ID)
	label = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(label)
	label = strings.ToLower(label)
	label = strings.Trim(label, "-")
	if label == "" {
		label = "model"
	}
	return fmt.Sprintf("official-%s", label)
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

