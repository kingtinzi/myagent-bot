package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/platformapi"
)

func RegisterAppPlatformAPI(mux *http.ServeMux, absPath string) {
	mux.HandleFunc("GET /api/app/session", func(w http.ResponseWriter, r *http.Request) {
		store := sessionStoreForConfig(absPath)
		session, err := store.Load()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
			return
		}
		client, err := platformClientForConfig(absPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
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
			resp["wallet_error"] = walletErr.Error()
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
}

func handleAppAuthMutation(w http.ResponseWriter, r *http.Request, absPath string, login bool) {
	var req platformapi.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	client, err := platformClientForConfig(absPath)
	if err != nil {
		writePlatformAPIError(w, absPath, err)
		return
	}
	var session platformapi.Session
	if login {
		session, err = client.Login(r.Context(), req)
	} else {
		session, err = client.SignUp(r.Context(), req)
	}
	if err != nil {
		writePlatformAPIError(w, absPath, err)
		return
	}
	if err := sessionStoreForConfig(absPath).Save(session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(platformapi.BrowserAuthResponse{Session: session.View()})
}

func platformClientForConfig(absPath string) (*platformapi.Client, error) {
	cfg, err := config.LoadConfig(absPath)
	if err != nil {
		return nil, err
	}
	return platformapi.NewClient(cfg.PlatformAPI.BaseURL), nil
}

func sessionStoreForConfig(absPath string) *platformapi.FileSessionStore {
	return platformapi.NewFileSessionStore(filepath.Dir(absPath))
}

func platformContext(
	w http.ResponseWriter,
	r *http.Request,
	absPath string,
) (*platformapi.Client, platformapi.Session, bool) {
	session, err := sessionStoreForConfig(absPath).Load()
	if err != nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return nil, platformapi.Session{}, false
	}
	client, err := platformClientForConfig(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
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
	http.Error(w, err.Error(), status)
}

type officialModelSyncResult struct {
	Added          int    `json:"added"`
	Updated        int    `json:"updated"`
	Removed        int    `json:"removed"`
	Total          int    `json:"total"`
	DefaultModel   string `json:"default_model,omitempty"`
	DefaultChanged bool   `json:"default_changed,omitempty"`
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

	for _, item := range cfg.ModelList {
		modelID, isOfficial := officialModelID(item.Model)
		if !isOfficial {
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
		if item != updated {
			result.Updated++
		}
		out = append(out, updated)
		seen[model.ID] = struct{}{}
	}

	imported := make([]string, 0, len(enabled))
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

	cfg.ModelList = out
	result.Total = len(out)
	if defaultRemoved || strings.TrimSpace(defaultModel) == "" {
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
