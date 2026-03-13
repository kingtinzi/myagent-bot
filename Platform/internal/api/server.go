package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
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

const maxJSONBodyBytes int64 = 1 << 20

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

	admin := func(h http.Handler) http.Handler {
		return s.authMiddleware(s.adminMiddleware(h))
	}
	user := func(h http.Handler) http.Handler {
		return s.authMiddleware(h)
	}

	s.mux.Handle("GET /admin/runtime-config", admin(http.HandlerFunc(s.handleAdminRuntimeConfigGet)))
	s.mux.Handle("PUT /admin/runtime-config", admin(http.HandlerFunc(s.handleAdminRuntimeConfigPut)))
	s.mux.Handle("GET /admin/models", admin(http.HandlerFunc(s.handleOfficialModels)))
	s.mux.Handle("GET /admin/model-routes", admin(http.HandlerFunc(s.handleAdminModelRoutes)))
	s.mux.Handle("PUT /admin/model-routes", admin(http.HandlerFunc(s.handleAdminModelRoutesPut)))
	s.mux.Handle("GET /admin/pricing-rules", admin(http.HandlerFunc(s.handleAdminPricingRules)))
	s.mux.Handle("PUT /admin/pricing-rules", admin(http.HandlerFunc(s.handleAdminPricingRulesPut)))
	s.mux.Handle("GET /admin/agreement-versions", admin(http.HandlerFunc(s.handleAdminAgreementVersions)))
	s.mux.Handle("PUT /admin/agreement-versions", admin(http.HandlerFunc(s.handleAdminAgreementVersionsPut)))
	s.mux.Handle("GET /admin/users", admin(http.HandlerFunc(s.handleAdminUsers)))
	s.mux.Handle("GET /admin/orders", admin(http.HandlerFunc(s.handleAdminOrders)))
	s.mux.Handle("POST /admin/orders/{id}/reconcile", admin(http.HandlerFunc(s.handleAdminOrderReconcile)))
	s.mux.Handle("POST /admin/orders/reconcile-pending", admin(http.HandlerFunc(s.handleAdminReconcilePendingOrders)))
	s.mux.Handle("GET /admin/wallet-adjustments", admin(http.HandlerFunc(s.handleAdminWalletAdjustments)))
	s.mux.Handle("GET /admin/audit-logs", admin(http.HandlerFunc(s.handleAdminAuditLogs)))
	s.mux.Handle("GET /admin/refund-requests", admin(http.HandlerFunc(s.handleAdminRefundRequests)))
	s.mux.Handle("POST /admin/refund-requests/{id}/approve", admin(http.HandlerFunc(s.handleAdminRefundApprove)))
	s.mux.Handle("POST /admin/refund-requests/{id}/reject", admin(http.HandlerFunc(s.handleAdminRefundReject)))
	s.mux.Handle("POST /admin/refund-requests/{id}/settle", admin(http.HandlerFunc(s.handleAdminRefundSettle)))
	s.mux.Handle("GET /admin/infringement-reports", admin(http.HandlerFunc(s.handleAdminInfringementReports)))
	s.mux.Handle("POST /admin/infringement-reports/{id}", admin(http.HandlerFunc(s.handleAdminInfringementReportUpdate)))
	s.mux.Handle("GET /admin/data-retention-policies", admin(http.HandlerFunc(s.handleAdminDataRetentionPolicies)))
	s.mux.Handle("PUT /admin/data-retention-policies", admin(http.HandlerFunc(s.handleAdminDataRetentionPoliciesPut)))
	s.mux.Handle("GET /admin/system-notices", admin(http.HandlerFunc(s.handleAdminSystemNotices)))
	s.mux.Handle("PUT /admin/system-notices", admin(http.HandlerFunc(s.handleAdminSystemNoticesPut)))
	s.mux.Handle("GET /admin/risk-rules", admin(http.HandlerFunc(s.handleAdminRiskRules)))
	s.mux.Handle("PUT /admin/risk-rules", admin(http.HandlerFunc(s.handleAdminRiskRulesPut)))

	s.mux.Handle("GET /official/models", user(http.HandlerFunc(s.handleEnabledOfficialModels)))
	s.mux.Handle("GET /official/access", user(http.HandlerFunc(s.handleOfficialAccessState)))
	s.mux.Handle("GET /agreements/current", user(http.HandlerFunc(s.handleAgreements)))
	s.mux.Handle("POST /agreements/accept", user(http.HandlerFunc(s.handleAgreementAccept)))
	s.mux.Handle("GET /wallet", user(http.HandlerFunc(s.handleWallet)))
	s.mux.Handle("GET /wallet/transactions", user(http.HandlerFunc(s.handleWalletTransactions)))
	s.mux.Handle("POST /wallet/orders", user(http.HandlerFunc(s.handleCreateOrder)))
	s.mux.Handle("GET /wallet/orders/{id}", user(http.HandlerFunc(s.handleWalletOrder)))
	s.mux.Handle("GET /wallet/refund-requests", user(http.HandlerFunc(s.handleWalletRefundRequests)))
	s.mux.Handle("POST /wallet/refund-requests", user(http.HandlerFunc(s.handleCreateRefundRequest)))
	s.mux.Handle("GET /infringement-reports", user(http.HandlerFunc(s.handleInfringementReports)))
	s.mux.Handle("POST /infringement-reports", user(http.HandlerFunc(s.handleCreateInfringementReport)))
	s.mux.Handle("POST /chat/official", user(http.HandlerFunc(s.handleOfficialChat)))
	s.mux.HandleFunc("POST /payments/easypay/notify", s.handleEasyPayNotify)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.verifier == nil {
			http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
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
	user, ok := requireAuthUser(w, r)
	if !ok {
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

func (s *Server) handleOfficialAccessState(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	state, err := s.service.GetOfficialAccessState(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, state)
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
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.Save(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.runtime_config.updated",
			TargetType:  "runtime_config",
			TargetID:    "runtime_config",
			RiskLevel:   "high",
			Detail:      "updated runtime config",
		})
	}
	writeJSON(w, http.StatusOK, s.runtimeConfig.Snapshot())
}

func (s *Server) handleAdminModelRoutes(w http.ResponseWriter, r *http.Request) {
	state, ok := s.requireRuntimeConfig(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, state.OfficialRoutes)
}

func (s *Server) handleAdminModelRoutesPut(w http.ResponseWriter, r *http.Request) {
	state, ok := s.requireRuntimeConfig(w)
	if !ok {
		return
	}
	var routes = state.OfficialRoutes
	if err := decodeJSONBody(w, r, &routes); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	state.OfficialRoutes = routes
	if err := s.runtimeConfig.Save(state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.model_routes.updated",
			TargetType:  "model_routes",
			TargetID:    "official_routes",
			RiskLevel:   "high",
			Detail:      "updated official model routes",
		})
	}
	writeJSON(w, http.StatusOK, s.runtimeConfig.Snapshot().OfficialRoutes)
}

func (s *Server) handleAdminPricingRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ListPricingRules())
}

func (s *Server) handleAdminPricingRulesPut(w http.ResponseWriter, r *http.Request) {
	state, ok := s.requireRuntimeConfig(w)
	if !ok {
		return
	}
	var rules []service.PricingRule
	if err := decodeJSONBody(w, r, &rules); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	state.PricingRules = rules
	if err := s.runtimeConfig.Save(state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.pricing_rules.updated",
			TargetType:  "pricing_rules",
			TargetID:    "pricing_rules",
			RiskLevel:   "high",
			Detail:      "updated pricing rules",
		})
	}
	writeJSON(w, http.StatusOK, s.service.ListPricingRules())
}

func (s *Server) handleAdminAgreementVersions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.ListAgreementVersions(r.Context()))
}

func (s *Server) handleAdminAgreementVersionsPut(w http.ResponseWriter, r *http.Request) {
	state, ok := s.requireRuntimeConfig(w)
	if !ok {
		return
	}
	var docs []service.AgreementDocument
	if err := decodeJSONBody(w, r, &docs); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	state.Agreements = docs
	if err := s.runtimeConfig.Save(state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.agreement_versions.updated",
			TargetType:  "agreement_versions",
			TargetID:    "agreements",
			RiskLevel:   "high",
			Detail:      "updated agreement versions",
		})
	}
	writeJSON(w, http.StatusOK, s.service.ListAgreementVersions(r.Context()))
}

func (s *Server) handleAgreementAccept(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Agreements []service.AgreementDocument `json:"agreements"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	source := service.AgreementAcceptanceSource{
		ClientVersion: strings.TrimSpace(r.Header.Get("X-Client-Version")),
		RemoteAddr:    strings.TrimSpace(r.RemoteAddr),
		DeviceSummary: strings.TrimSpace(r.UserAgent()),
	}
	if err := s.service.RecordAgreementAcceptances(r.Context(), user.ID, req.Agreements, source); err != nil {
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
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	if err := s.service.EnsureRechargeAgreementsAccepted(r.Context(), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	var input service.CreateOrderInput
	if err := decodeJSONBody(w, r, &input); err != nil {
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

func (s *Server) handleWalletOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	order, err := s.service.GetOrder(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleWalletTransactions(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	items, err := s.service.ListTransactions(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleWalletRefundRequests(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	items, err := s.service.ListRefundRequests(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateRefundRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		OrderID   string `json:"order_id"`
		AmountFen int64  `json:"amount_fen"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	item, err := s.service.CreateRefundRequest(r.Context(), user.ID, req.AmountFen, req.OrderID, req.Reason)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrInvalidAmount) {
			status = http.StatusBadRequest
		} else if errors.Is(err, service.ErrRefundNotAllowed) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleOfficialChat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var input platformapi.ChatProxyRequest
	if err := decodeJSONBody(w, r, &input); err != nil {
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
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	session, err := fn(r.Context(), req)
	if err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadGateway, "authentication service unavailable")
		http.Error(w, message, status)
		return
	}
	writeJSON(w, http.StatusOK, platformapi.AuthResponse{Session: session})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListOrders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminOrderReconcile(w http.ResponseWriter, r *http.Request) {
	order, changed, err := s.service.ReconcileRechargeOrder(r.Context(), r.PathValue("id"))
	if err != nil {
		statusCode := http.StatusBadGateway
		if errors.Is(err, payments.ErrOperationNotSupported) {
			statusCode = http.StatusNotImplemented
		}
		http.Error(w, err.Error(), statusCode)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"changed": changed,
		"order":   order,
	})
}

func (s *Server) handleAdminReconcilePendingOrders(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ReconcilePendingRechargeOrders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(items),
		"orders": items,
	})
}

func (s *Server) handleAdminWalletAdjustments(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListWalletAdjustments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListAuditLogs(r.Context(), service.AuditLogFilter{
		Action:      strings.TrimSpace(r.URL.Query().Get("action")),
		TargetType:  strings.TrimSpace(r.URL.Query().Get("target_type")),
		ActorUserID: strings.TrimSpace(r.URL.Query().Get("actor_user_id")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminRefundRequests(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListRefundRequests(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminRefundApprove(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		ReviewNote     string `json:"review_note"`
		RefundProvider string `json:"refund_provider"`
	}
	if err := decodeOptionalJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	item, err := s.service.ApproveRefundRequest(r.Context(), r.PathValue("id"), service.RefundDecisionInput{
		ReviewNote:     req.ReviewNote,
		RefundProvider: req.RefundProvider,
		ReviewedBy:     adminUser.ID,
	})
	if err != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(err, payments.ErrOperationNotSupported) {
			statusCode = http.StatusNotImplemented
		}
		http.Error(w, err.Error(), statusCode)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAdminRefundReject(w http.ResponseWriter, r *http.Request) {
	s.handleAdminRefundDecision(w, r, "rejected")
}

func (s *Server) handleAdminRefundSettle(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		ReviewNote       string `json:"review_note"`
		RefundProvider   string `json:"refund_provider"`
		ExternalRefundID string `json:"external_refund_id"`
		ExternalStatus   string `json:"external_status"`
	}
	if err := decodeOptionalJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	item, err := s.service.MarkRefundSettled(r.Context(), r.PathValue("id"), service.RefundDecisionInput{
		ReviewNote:       req.ReviewNote,
		RefundProvider:   req.RefundProvider,
		ReviewedBy:       adminUser.ID,
		ExternalRefundID: req.ExternalRefundID,
		ExternalStatus:   req.ExternalStatus,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAdminRefundDecision(w http.ResponseWriter, r *http.Request, status string) {
	adminUser, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req struct {
		ReviewNote     string `json:"review_note"`
		RefundProvider string `json:"refund_provider"`
	}
	if err := decodeOptionalJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	item, err := s.service.ReviewRefundRequest(r.Context(), r.PathValue("id"), service.RefundDecisionInput{
		Status:         status,
		ReviewNote:     req.ReviewNote,
		RefundProvider: req.RefundProvider,
		ReviewedBy:     adminUser.ID,
	})
	if err != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(err, service.ErrRefundNotAllowed) {
			statusCode = http.StatusConflict
		}
		http.Error(w, err.Error(), statusCode)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleCreateInfringementReport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req service.InfringementReport
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.UserID = user.ID
	item, err := s.service.CreateInfringementReport(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleInfringementReports(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	items, err := s.service.ListInfringementReports(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminInfringementReports(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListInfringementReports(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminInfringementReportUpdate(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req service.InfringementUpdateInput
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.ReviewedBy = adminUser.ID
	item, err := s.service.UpdateInfringementReport(r.Context(), r.PathValue("id"), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAdminDataRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListDataRetentionPolicies(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminDataRetentionPoliciesPut(w http.ResponseWriter, r *http.Request) {
	var items []service.DataRetentionPolicy
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveDataRetentionPolicies(r.Context(), items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.data_retention_policies.updated",
			TargetType:  "data_retention_policies",
			TargetID:    "data_retention_policies",
			RiskLevel:   "medium",
			Detail:      "updated retention policies",
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminSystemNotices(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListSystemNotices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminSystemNoticesPut(w http.ResponseWriter, r *http.Request) {
	var items []service.SystemNotice
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveSystemNotices(r.Context(), items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.system_notices.updated",
			TargetType:  "system_notices",
			TargetID:    "system_notices",
			RiskLevel:   "medium",
			Detail:      "updated system notices",
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminRiskRules(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListRiskRules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminRiskRulesPut(w http.ResponseWriter, r *http.Request) {
	var items []service.RiskRule
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveRiskRules(r.Context(), items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.risk_rules.updated",
			TargetType:  "risk_rules",
			TargetID:    "risk_rules",
			RiskLevel:   "medium",
			Detail:      "updated risk rules",
		})
	}
	writeJSON(w, http.StatusOK, items)
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

func requireAuthUser(w http.ResponseWriter, r *http.Request) (AuthUser, bool) {
	user, err := authUserFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return AuthUser{}, false
	}
	return user, true
}

func (s *Server) requireRuntimeConfig(w http.ResponseWriter) (runtimeconfig.State, bool) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return runtimeconfig.State{}, false
	}
	return s.runtimeConfig.Snapshot(), true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) servicePaymentProvider() payments.Provider {
	return s.service.PaymentProvider()
}

func statusAndMessageFromError(err error, fallbackStatus int, fallbackMessage string) (int, string) {
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
	if strings.TrimSpace(fallbackMessage) != "" {
		return fallbackStatus, fallbackMessage
	}
	return fallbackStatus, err.Error()
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	return json.NewDecoder(r.Body).Decode(target)
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	return json.NewDecoder(r.Body).Decode(target)
}
