package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/sipeed/pinchbot/pkg/platformapi"

	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/runtimeconfig"
	"openclaw/platform/internal/service"
)

type AuthUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type TokenVerifier interface {
	Verify(ctx context.Context, bearerToken string) (AuthUser, error)
}

type AuthBridge interface {
	Login(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error)
	SignUp(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error)
}

type Server struct {
	service       *service.Service
	verifier      TokenVerifier
	authBridge    AuthBridge
	runtimeConfig *runtimeconfig.Manager
	mux           *http.ServeMux
}

//go:embed admin_index.html
var adminUI embed.FS

func NewServer(
	svc *service.Service,
	verifier TokenVerifier,
	auth AuthBridge,
	runtimeConfig *runtimeconfig.Manager,
) http.Handler {
	s := &Server{
		service:       svc,
		verifier:      verifier,
		authBridge:    auth,
		runtimeConfig: runtimeConfig,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("POST /auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /auth/signup", s.handleSignup)
	s.mux.HandleFunc("GET /admin", s.handleAdminUI)
	s.mux.Handle("GET /admin/runtime-config", s.authMiddleware(s.adminMiddleware(http.HandlerFunc(s.handleAdminRuntimeConfigGet))))
	s.mux.Handle("PUT /admin/runtime-config", s.authMiddleware(s.adminMiddleware(http.HandlerFunc(s.handleAdminRuntimeConfigPut))))
	s.mux.Handle("GET /admin/models", s.authMiddleware(s.adminMiddleware(http.HandlerFunc(s.handleOfficialModels))))
	s.mux.Handle("GET /official/models", s.authMiddleware(http.HandlerFunc(s.handleEnabledOfficialModels)))
	s.mux.Handle("GET /agreements/current", s.authMiddleware(http.HandlerFunc(s.handleAgreements)))
	s.mux.Handle("POST /agreements/accept", s.authMiddleware(http.HandlerFunc(s.handleAgreementAccept)))
	s.mux.Handle("GET /wallet", s.authMiddleware(http.HandlerFunc(s.handleWallet)))
	s.mux.Handle("GET /wallet/transactions", s.authMiddleware(http.HandlerFunc(s.handleWalletTransactions)))
	s.mux.Handle("POST /wallet/orders", s.authMiddleware(http.HandlerFunc(s.handleCreateOrder)))
	s.mux.Handle("POST /chat/official", s.authMiddleware(http.HandlerFunc(s.handleOfficialChat)))
	s.mux.HandleFunc("POST /payments/easypay/notify", s.handleEasyPayNotify)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.verifier == nil {
			http.Error(w, "auth verifier not configured", http.StatusServiceUnavailable)
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		user, err := s.verifier.Verify(r.Context(), strings.TrimSpace(header[7:]))
		if err != nil {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
	})
}

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	wallet, err := s.service.GetWallet(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, wallet)
}

func (s *Server) handleOfficialModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ListOfficialModels(r.Context()))
}

func (s *Server) handleEnabledOfficialModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ListEnabledOfficialModels(r.Context()))
}

func (s *Server) handleAgreements(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ListAgreements(r.Context()))
}

func (s *Server) handleAdminRuntimeConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, s.runtimeConfig.Snapshot())
}

func (s *Server) handleAdminRuntimeConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	var req runtimeconfig.State
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.Save(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.runtimeConfig.Snapshot())
}

func (s *Server) handleAgreementAccept(w http.ResponseWriter, r *http.Request) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var req struct {
		Agreements []service.AgreementDocument `json:"agreements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.RecordAgreementAcceptances(r.Context(), user.ID, req.Agreements); err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, service.ErrInvalidAgreement), errors.Is(err, service.ErrUnknownAgreement):
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err := s.service.EnsureRechargeAgreementsAccepted(r.Context(), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	var input service.CreateOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	order, err := s.service.CreateRechargeOrder(r.Context(), user.ID, input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidAmount) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) handleWalletTransactions(w http.ResponseWriter, r *http.Request) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	items, err := s.service.ListTransactions(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleOfficialChat(w http.ResponseWriter, r *http.Request) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input platformapi.ChatProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := s.service.ProxyOfficialChatRequest(r.Context(), user.ID, input)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, service.ErrUnknownModel), errors.Is(err, service.ErrModelDisabled):
			status = http.StatusForbidden
		case errors.Is(err, service.ErrInsufficientFunds):
			status = http.StatusPaymentRequired
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEasyPayNotify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form payload", http.StatusBadRequest)
		return
	}
	callbackProvider := s.servicePaymentProvider()
	if callbackProvider == nil {
		http.Error(w, "payment provider not configured", http.StatusServiceUnavailable)
		return
	}
	result, err := callbackProvider.VerifyCallback(r.Context(), r.Form)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if result.Paid {
		if _, err := s.service.HandleSuccessfulRechargeCallback(
			r.Context(),
			result.OrderID,
			callbackProvider.Name(),
			result.ExternalOrderID,
			result.AmountFen,
		); err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, service.ErrCallbackAmount) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("success"))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.handleAuthMutation(w, r, func(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
		if s.authBridge == nil {
			return platformapi.Session{}, errors.New("auth bridge not configured")
		}
		return s.authBridge.Login(ctx, req)
	})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	s.handleAuthMutation(w, r, func(ctx context.Context, req platformapi.AuthRequest) (platformapi.Session, error) {
		if s.authBridge == nil {
			return platformapi.Session{}, errors.New("auth bridge not configured")
		}
		return s.authBridge.SignUp(ctx, req)
	})
}

func (s *Server) handleAuthMutation(
	w http.ResponseWriter,
	r *http.Request,
	fn func(context.Context, platformapi.AuthRequest) (platformapi.Session, error),
) {
	var req platformapi.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	session, err := fn(r.Context(), req)
	if err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadGateway)
		http.Error(w, message, status)
		return
	}
	writeJSON(w, http.StatusOK, platformapi.AuthResponse{Session: session})
}

func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	file, err := fs.ReadFile(adminUI, "admin_index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(file)
}

func (s *Server) adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := authUserFromContext(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ok, err := s.service.IsAdminUser(r.Context(), user.ID, user.Email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authUserKey struct{}

func authUserFromContext(ctx context.Context) (AuthUser, error) {
	user, ok := ctx.Value(authUserKey{}).(AuthUser)
	if !ok || strings.TrimSpace(user.ID) == "" {
		return AuthUser{}, errors.New("missing auth user")
	}
	return user, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) servicePaymentProvider() payments.Provider {
	return s.service.PaymentProvider()
}

func statusAndMessageFromError(err error, fallbackStatus int) (int, string) {
	var apiErr *platformapi.APIError
	if errors.As(err, &apiErr) {
		message := strings.TrimSpace(apiErr.Message)
		if message == "" {
			message = err.Error()
		}
		if apiErr.StatusCode > 0 {
			return apiErr.StatusCode, message
		}
		return fallbackStatus, message
	}
	return fallbackStatus, err.Error()
}
