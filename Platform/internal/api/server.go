package api

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"

	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/revisiontoken"
	"openclaw/platform/internal/runtimeconfig"
	"openclaw/platform/internal/service"
	"openclaw/platform/internal/upstream"
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
const adminSessionCookieName = "pinchbot_admin_session"

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
	s.mux.HandleFunc("POST /admin/session/login", s.handleAdminSessionLogin)
	s.mux.HandleFunc("POST /admin/session/logout", s.handleAdminSessionLogout)

	admin := func(capability string, h http.Handler) http.Handler {
		wrapped := h
		if strings.TrimSpace(capability) != "" {
			wrapped = s.adminCapabilityMiddleware(capability, wrapped)
		}
		return s.adminAuthMiddleware(s.adminMiddleware(wrapped))
	}
	user := func(h http.Handler) http.Handler {
		return s.userAuthMiddleware(h)
	}
	s.mux.Handle("POST /auth/logout", user(http.HandlerFunc(s.handleLogout)))

	s.mux.Handle("GET /admin/session", admin("", http.HandlerFunc(s.handleAdminSession)))
	s.mux.Handle("GET /admin/me", admin("", http.HandlerFunc(s.handleAdminMe)))
	s.mux.Handle("GET /admin/dashboard", admin(service.AdminCapabilityDashboardRead, http.HandlerFunc(s.handleAdminDashboard)))
	s.mux.Handle("GET /admin/runtime-config", admin(service.AdminCapabilityRuntimeRead, http.HandlerFunc(s.handleAdminRuntimeConfigGet)))
	s.mux.Handle("PUT /admin/runtime-config", admin(service.AdminCapabilityRuntimeWrite, http.HandlerFunc(s.handleAdminRuntimeConfigPut)))
	s.mux.Handle("GET /admin/models", admin(service.AdminCapabilityModelsRead, http.HandlerFunc(s.handleOfficialModels)))
	s.mux.Handle("PUT /admin/models", admin(service.AdminCapabilityModelsWrite, http.HandlerFunc(s.handleAdminModelsPut)))
	s.mux.Handle("GET /admin/model-routes", admin(service.AdminCapabilityRoutesRead, http.HandlerFunc(s.handleAdminModelRoutes)))
	s.mux.Handle("PUT /admin/model-routes", admin(service.AdminCapabilityRoutesWrite, http.HandlerFunc(s.handleAdminModelRoutesPut)))
	s.mux.Handle("GET /admin/pricing-rules", admin(service.AdminCapabilityPricingRead, http.HandlerFunc(s.handleAdminPricingRules)))
	s.mux.Handle("PUT /admin/pricing-rules", admin(service.AdminCapabilityPricingWrite, http.HandlerFunc(s.handleAdminPricingRulesPut)))
	s.mux.Handle("GET /admin/agreement-versions", admin(service.AdminCapabilityAgreementsRead, http.HandlerFunc(s.handleAdminAgreementVersions)))
	s.mux.Handle("PUT /admin/agreement-versions", admin(service.AdminCapabilityAgreementsWrite, http.HandlerFunc(s.handleAdminAgreementVersionsPut)))
	s.mux.Handle("GET /admin/users", admin(service.AdminCapabilityUsersRead, http.HandlerFunc(s.handleAdminUsers)))
	s.mux.Handle("GET /admin/users/{id}/overview", admin(service.AdminCapabilityUsersRead, http.HandlerFunc(s.handleAdminUserOverview)))
	s.mux.Handle("GET /admin/users/{id}/wallet-transactions", admin(service.AdminCapabilityWalletRead, http.HandlerFunc(s.handleAdminUserWalletTransactions)))
	s.mux.Handle("GET /admin/users/{id}/orders", admin(service.AdminCapabilityOrdersRead, http.HandlerFunc(s.handleAdminUserOrders)))
	s.mux.Handle("GET /admin/users/{id}/agreements", admin(service.AdminCapabilityAgreementsRead, http.HandlerFunc(s.handleAdminUserAgreements)))
	s.mux.Handle("GET /admin/users/{id}/usage", admin(service.AdminCapabilityUsageRead, http.HandlerFunc(s.handleAdminUserUsage)))
	s.mux.Handle("GET /admin/operators", admin(service.AdminCapabilityOperatorsRead, http.HandlerFunc(s.handleAdminOperators)))
	s.mux.Handle("PUT /admin/operators/{email}", admin(service.AdminCapabilityOperatorsWrite, http.HandlerFunc(s.handleAdminOperatorPut)))
	s.mux.Handle("GET /admin/orders", admin(service.AdminCapabilityOrdersRead, http.HandlerFunc(s.handleAdminOrders)))
	s.mux.Handle("POST /admin/orders/{id}/reconcile", admin(service.AdminCapabilityOrdersWrite, http.HandlerFunc(s.handleAdminOrderReconcile)))
	s.mux.Handle("POST /admin/orders/reconcile-pending", admin(service.AdminCapabilityOrdersWrite, http.HandlerFunc(s.handleAdminReconcilePendingOrders)))
	s.mux.Handle("GET /admin/wallet-adjustments", admin(service.AdminCapabilityWalletRead, http.HandlerFunc(s.handleAdminWalletAdjustments)))
	s.mux.Handle("POST /admin/manual-recharges", admin(service.AdminCapabilityWalletWrite, http.HandlerFunc(s.handleAdminManualRechargeCreate)))
	s.mux.Handle("POST /admin/wallet-adjustments", admin(service.AdminCapabilityWalletWrite, http.HandlerFunc(s.handleAdminWalletAdjustmentCreate)))
	s.mux.Handle("GET /admin/audit-logs", admin(service.AdminCapabilityAuditRead, http.HandlerFunc(s.handleAdminAuditLogs)))
	s.mux.Handle("GET /admin/refund-requests", admin(service.AdminCapabilityRefundsRead, http.HandlerFunc(s.handleAdminRefundRequests)))
	s.mux.Handle("POST /admin/refund-requests/{id}/approve", admin(service.AdminCapabilityRefundsReview, http.HandlerFunc(s.handleAdminRefundApprove)))
	s.mux.Handle("POST /admin/refund-requests/{id}/reject", admin(service.AdminCapabilityRefundsReview, http.HandlerFunc(s.handleAdminRefundReject)))
	s.mux.Handle("POST /admin/refund-requests/{id}/settle", admin(service.AdminCapabilityRefundsReview, http.HandlerFunc(s.handleAdminRefundSettle)))
	s.mux.Handle("GET /admin/infringement-reports", admin(service.AdminCapabilityInfringementRead, http.HandlerFunc(s.handleAdminInfringementReports)))
	s.mux.Handle("POST /admin/infringement-reports/{id}", admin(service.AdminCapabilityInfringementReview, http.HandlerFunc(s.handleAdminInfringementReportUpdate)))
	s.mux.Handle("GET /admin/data-retention-policies", admin(service.AdminCapabilityRetentionRead, http.HandlerFunc(s.handleAdminDataRetentionPolicies)))
	s.mux.Handle("PUT /admin/data-retention-policies", admin(service.AdminCapabilityRetentionWrite, http.HandlerFunc(s.handleAdminDataRetentionPoliciesPut)))
	s.mux.Handle("GET /admin/system-notices", admin(service.AdminCapabilityNoticesRead, http.HandlerFunc(s.handleAdminSystemNotices)))
	s.mux.Handle("PUT /admin/system-notices", admin(service.AdminCapabilityNoticesWrite, http.HandlerFunc(s.handleAdminSystemNoticesPut)))
	s.mux.Handle("GET /admin/risk-rules", admin(service.AdminCapabilityRiskRead, http.HandlerFunc(s.handleAdminRiskRules)))
	s.mux.Handle("PUT /admin/risk-rules", admin(service.AdminCapabilityRiskWrite, http.HandlerFunc(s.handleAdminRiskRulesPut)))

	s.mux.Handle("GET /official/models", user(http.HandlerFunc(s.handleEnabledOfficialModels)))
	s.mux.Handle("GET /me", user(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("GET /official/access", user(http.HandlerFunc(s.handleOfficialAccessState)))
	s.mux.HandleFunc("GET /agreements/current", s.handleAgreements)
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
	s.mux.HandleFunc("GET /payments/easypay/return", s.handlePaymentReturn)
	s.mux.HandleFunc("GET /payments/alimpay/notify", s.handleEasyPayNotify)
	s.mux.HandleFunc("POST /payments/alimpay/notify", s.handleEasyPayNotify)
	s.mux.HandleFunc("GET /payments/alimpay/return", s.handlePaymentReturn)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) userAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.authenticateRequest(w, r, false, next)
	})
}

func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.authenticateAdminRequest(w, r, next)
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
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListOfficialModels(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminModelsPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var models []service.OfficialModel
	if err := decodeJSONBody(w, r, &models); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.SaveModelsWithRevision(expectedRevision, models); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadRequest, "")
		http.Error(w, message, status)
		return
	}
	if user, err := authUserFromContext(r.Context()); err == nil {
		_ = s.service.RecordAdminAudit(r.Context(), service.AdminAuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Action:      "admin.models.updated",
			TargetType:  "official_models",
			TargetID:    "official_models",
			RiskLevel:   "high",
			Detail:      "updated official model catalog",
		})
	}
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListOfficialModels(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, platformapi.BrowserAuthResponse{
		Session: platformapi.SessionView{
			UserID:   user.ID,
			Username: s.lookupMirroredUsername(r.Context(), user.ID),
			Email:    user.Email,
		},
	})
}

func (s *Server) handleAdminSessionLogin(w http.ResponseWriter, r *http.Request) {
	if s.authBridge == nil {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("auth bridge not configured"), http.StatusServiceUnavailable)
		return
	}
	if s.verifier == nil {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("authentication service unavailable"), http.StatusServiceUnavailable)
		return
	}
	var req platformapi.AuthRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("invalid json"), http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	session, err := s.authBridge.Login(r.Context(), platformapi.AuthRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadGateway, "authentication service unavailable")
		http.Error(w, message, status)
		return
	}
	accessToken := strings.TrimSpace(session.AccessToken)
	if accessToken == "" {
		s.clearAdminSessionCookie(w, r)
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("login did not return an administrator session"), http.StatusBadGateway)
		return
	}
	user, err := s.verifier.Verify(r.Context(), accessToken)
	if err != nil {
		s.clearAdminSessionCookie(w, r)
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("failed to verify administrator session"), http.StatusBadGateway)
		return
	}
	s.mirrorUserIdentity(r.Context(), user.ID, user.Email, session.Username)
	operator, err := s.service.GetAdminOperator(r.Context(), user.ID, user.Email)
	if err != nil {
		s.clearAdminSessionCookie(w, r)
		if errors.Is(err, service.ErrAdminAccessDenied) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setAdminSessionCookie(w, r, session)
	writeJSON(w, http.StatusOK, adminSessionResponse{
		User:     user,
		Operator: operator,
	})
}

func (s *Server) handleAdminSessionLogout(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(adminSessionCookieName); err == nil {
		if err := validateAdminSessionOrigin(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}
	s.clearAdminSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	s.handleAdminMe(w, r)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuthUser(w, r); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	state := s.runtimeConfig.Snapshot()
	revision, err := runtimeconfig.RevisionForState(state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithCustomRevision(w, http.StatusOK, runtimeconfig.RedactState(state), revision); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminRuntimeConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var req runtimeconfig.State
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.SaveWithRevision(expectedRevision, req); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadRequest, "")
		http.Error(w, message, status)
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
	state := s.runtimeConfig.Snapshot()
	revision, err := runtimeconfig.RevisionForState(state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithCustomRevision(w, http.StatusOK, runtimeconfig.RedactState(state), revision); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminModelRoutes(w http.ResponseWriter, r *http.Request) {
	state, ok := s.requireRuntimeConfig(w)
	if !ok {
		return
	}
	revision, err := runtimeconfig.RevisionForRoutes(state.OfficialRoutes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithCustomRevision(w, http.StatusOK, runtimeconfig.RedactState(state).OfficialRoutes, revision); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminModelRoutesPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var routes []upstream.OfficialRoute
	if err := decodeJSONBody(w, r, &routes); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.SaveRoutesWithRevision(expectedRevision, routes); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadRequest, "")
		http.Error(w, message, status)
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
	state := s.runtimeConfig.Snapshot()
	revision, err := runtimeconfig.RevisionForRoutes(state.OfficialRoutes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithCustomRevision(w, http.StatusOK, runtimeconfig.RedactState(state).OfficialRoutes, revision); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminPricingRules(w http.ResponseWriter, r *http.Request) {
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListPricingRules()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminPricingRulesPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var rules []service.PricingRule
	if err := decodeJSONBody(w, r, &rules); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.SavePricingRulesWithRevision(expectedRevision, rules); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadRequest, "")
		http.Error(w, message, status)
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
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListPricingRules()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminAgreementVersions(w http.ResponseWriter, r *http.Request) {
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListAgreementVersions(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminAgreementVersionsPut(w http.ResponseWriter, r *http.Request) {
	if s.runtimeConfig == nil {
		http.Error(w, "runtime config not configured", http.StatusServiceUnavailable)
		return
	}
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var docs []service.AgreementDocument
	if err := decodeJSONBody(w, r, &docs); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.runtimeConfig.SaveAgreementsWithRevision(expectedRevision, docs); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadRequest, "")
		http.Error(w, message, status)
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
	if err := writeJSONWithRevision(w, http.StatusOK, s.service.ListAgreementVersions(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	items, err := s.service.ListRefundRequests(r.Context(), service.RefundRequestFilter{
		UserID:  user.ID,
		OrderID: strings.TrimSpace(r.URL.Query().Get("order_id")),
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:   parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:  parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
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
		message := "official model request failed, please retry later"
		switch {
		case errors.Is(err, service.ErrUnknownModel), errors.Is(err, service.ErrModelDisabled):
			status = http.StatusForbidden
			message = err.Error()
		case errors.Is(err, service.ErrInsufficientFunds):
			status = http.StatusPaymentRequired
			message = err.Error()
		}
		http.Error(w, message, status)
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

func (s *Server) handlePaymentReturn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "支付回跳参数无效", http.StatusBadRequest)
		return
	}
	orderID := strings.TrimSpace(r.Form.Get("out_trade_no"))
	if orderID == "" {
		http.Error(w, "缺少订单号参数 out_trade_no", http.StatusBadRequest)
		return
	}

	providerName := "支付"
	if strings.Contains(strings.ToLower(r.URL.Path), "alimpay") {
		providerName = "AliMPay"
	} else if strings.Contains(strings.ToLower(r.URL.Path), "easypay") {
		providerName = "EasyPay"
	}

	provider := s.servicePaymentProvider()
	if provider == nil {
		http.Error(w, "支付通道未配置", http.StatusServiceUnavailable)
		return
	}

	if strings.TrimSpace(r.Form.Get("sign")) != "" && strings.TrimSpace(r.Form.Get("trade_status")) != "" {
		if result, err := provider.VerifyCallback(r.Context(), r.Form); err == nil && result.Paid {
			if _, err := s.service.HandleSuccessfulRechargeCallback(
				r.Context(),
				result.OrderID,
				provider.Name(),
				result.ExternalOrderID,
				result.AmountFen,
			); err != nil && !errors.Is(err, service.ErrCallbackAmount) {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
	}

	order, _, err := s.service.ReconcileRechargeOrder(r.Context(), orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	statusLabel := map[string]string{
		"paid":     "支付成功",
		"pending":  "支付处理中",
		"refunded": "已退款",
		"closed":   "订单已关闭",
	}[strings.ToLower(strings.TrimSpace(order.Status))]
	if statusLabel == "" {
		statusLabel = "订单状态：" + order.Status
	}
	detail := "请返回应用刷新钱包或订单状态。"
	if strings.EqualFold(order.Status, "paid") {
		detail = "平台已确认到账，你可以返回应用继续使用。"
	}
	s.writePaymentReturnHTML(w, providerName, order, statusLabel, detail)
}

func (s *Server) writePaymentReturnHTML(w http.ResponseWriter, providerName string, order service.RechargeOrder, statusLabel, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := html.EscapeString(providerName + " 支付结果")
	orderID := html.EscapeString(order.ID)
	statusText := html.EscapeString(statusLabel)
	detailText := html.EscapeString(detail)
	amountText := html.EscapeString(fmt.Sprintf("¥%.2f", float64(order.AmountFen)/100))
	providerStatus := html.EscapeString(strings.TrimSpace(order.ProviderStatus))
	if providerStatus == "" {
		providerStatus = "暂无"
	}
	externalID := html.EscapeString(strings.TrimSpace(order.ExternalID))
	if externalID == "" {
		externalID = "暂无"
	}
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #0f172a; color: #e2e8f0; margin: 0; padding: 24px; }
    .card { max-width: 560px; margin: 4vh auto; background: #111827; border: 1px solid #334155; border-radius: 16px; padding: 24px; box-shadow: 0 18px 60px rgba(15,23,42,.45); }
    h1 { margin: 0 0 12px; font-size: 24px; }
    p { color: #cbd5e1; line-height: 1.6; }
    dl { margin: 20px 0 0; display: grid; grid-template-columns: 120px 1fr; gap: 10px 14px; }
    dt { color: #94a3b8; }
    dd { margin: 0; word-break: break-all; }
    .badge { display: inline-block; padding: 6px 10px; border-radius: 999px; background: #1d4ed8; color: #eff6ff; font-weight: 600; margin-bottom: 14px; }
  </style>
</head>
<body>
  <main class="card">
    <div class="badge">%s</div>
    <h1>%s</h1>
    <p>%s</p>
    <dl>
      <dt>订单号</dt><dd>%s</dd>
      <dt>金额</dt><dd>%s</dd>
      <dt>平台状态</dt><dd>%s</dd>
      <dt>上游状态</dt><dd>%s</dd>
      <dt>外部流水</dt><dd>%s</dd>
    </dl>
  </main>
</body>
</html>`, title, title, statusText, detailText, orderID, amountText, html.EscapeString(order.Status), providerStatus, externalID)
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
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("invalid json"), http.StatusBadRequest)
		return
	}
	authReq := platformapi.AuthRequest{
		Email:    strings.TrimSpace(req.Email),
		Password: req.Password,
	}
	if !platformapi.IsLikelyValidEmailAddress(authReq.Email) {
		http.Error(w, platformapi.InvalidEmailFormatMessage, http.StatusBadRequest)
		return
	}
	var signupAgreements []service.AgreementDocument
	if r.URL.Path == "/auth/signup" {
		authReq.Username = strings.TrimSpace(req.Username)
		if authReq.Username == "" {
			http.Error(w, "请输入用户名", http.StatusBadRequest)
			return
		}
		validated, err := s.service.ValidateRequiredAuthAgreements(
			r.Context(),
			toServiceAgreementDocuments(platformapi.FilterAuthAgreements(req.Agreements)),
		)
		if err != nil {
			http.Error(w, platformapi.NormalizeUserFacingErrorMessage(err.Error()), http.StatusBadRequest)
			return
		}
		signupAgreements = validated
	}

	session, err := fn(r.Context(), authReq)
	if err != nil {
		status, message := statusAndMessageFromError(err, http.StatusBadGateway, "authentication service unavailable")
		http.Error(w, message, status)
		return
	}
	session.AccessToken = strings.TrimSpace(session.AccessToken)
	if session.AccessToken == "" {
		http.Error(w, platformapi.NormalizeUserFacingErrorMessage("authentication service did not return a valid session"), http.StatusBadGateway)
		return
	}
	s.mirrorUserIdentity(r.Context(), session.UserID, session.Email, firstNonEmptyString(session.Username, authReq.Username))
	agreementSyncRequired := false
	agreementWarning := ""
	if len(signupAgreements) > 0 {
		source := service.AgreementAcceptanceSource{
			ClientVersion: strings.TrimSpace(r.Header.Get("X-Client-Version")),
			RemoteAddr:    strings.TrimSpace(r.RemoteAddr),
			DeviceSummary: strings.TrimSpace(r.UserAgent()),
		}
		if err := s.service.RecordAgreementAcceptances(r.Context(), session.UserID, signupAgreements, source); err != nil {
			agreementSyncRequired = true
			agreementWarning = platformapi.NormalizeUserFacingErrorMessage("signup succeeded, but agreement sync must be retried before recharge")
		}
	}
	writeJSON(w, http.StatusOK, platformapi.AuthResponse{
		Session:               session,
		AgreementSyncRequired: agreementSyncRequired,
		Warning:               agreementWarning,
	})
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	operator, ok := requireAdminOperator(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, adminSessionResponse{User: user, Operator: operator})
}

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	windowDays := parsePositiveInt(r.URL.Query().Get("since_days"))
	dashboard, err := s.service.GetAdminDashboardForWindow(r.Context(), windowDays)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) handleAdminUserOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.service.GetAdminUserOverview(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if operator, err := adminOperatorFromContext(r.Context()); err == nil {
		overview = s.service.RedactAdminUserOverview(overview, operator)
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleAdminUserWalletTransactions(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListUserWalletTransactions(
		r.Context(),
		r.PathValue("id"),
		parsePositiveInt(r.URL.Query().Get("limit")),
		parseNonNegativeInt(r.URL.Query().Get("offset")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminUserOrders(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListOrders(r.Context(), service.RechargeOrderFilter{
		UserID:   r.PathValue("id"),
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Provider: strings.TrimSpace(r.URL.Query().Get("provider")),
		Limit:    parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:   parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminUserAgreements(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListUserAgreementAcceptances(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminUserUsage(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListChatUsageRecords(r.Context(), service.ChatUsageRecordFilter{
		UserID:    r.PathValue("id"),
		ModelID:   strings.TrimSpace(r.URL.Query().Get("model_id")),
		SinceUnix: parseUnixSeconds(r.URL.Query().Get("since_unix")),
		Limit:     parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:    parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminOperators(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListAdminOperators(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminOperatorPut(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req service.AdminOperator
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(r.PathValue("email")))
	item, err := s.service.SaveAdminOperator(r.Context(), service.AdminActor{
		UserID: adminUser.ID,
		Email:  adminUser.Email,
	}, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if keyword == "" {
		keyword = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	items, err := s.service.ListUsers(r.Context(), service.UserSummaryFilter{
		UserID:  strings.TrimSpace(r.URL.Query().Get("user_id")),
		Email:   strings.TrimSpace(r.URL.Query().Get("email")),
		Keyword: keyword,
		Limit:   parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:  parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListOrders(r.Context(), service.RechargeOrderFilter{
		UserID:      strings.TrimSpace(r.URL.Query().Get("user_id")),
		UserKeyword: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("user_keyword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Provider:    strings.TrimSpace(r.URL.Query().Get("provider")),
		Limit:       parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:      parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
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
	items, err := s.service.ListWalletAdjustments(r.Context(), service.WalletAdjustmentFilter{
		UserID:        strings.TrimSpace(r.URL.Query().Get("user_id")),
		UserKeyword:   firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("user_keyword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		Kind:          strings.TrimSpace(r.URL.Query().Get("kind")),
		ReferenceType: strings.TrimSpace(r.URL.Query().Get("reference_type")),
		Limit:         parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:        parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminWalletAdjustmentCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req service.AdminWalletAdjustmentInput
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	wallet, replayed, err := s.service.ApplyAdminWalletAdjustment(r.Context(), service.AdminActor{
		UserID: user.ID,
		Email:  user.Email,
	}, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAmount), errors.Is(err, service.ErrInvalidRequestID):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, service.ErrInsufficientFunds):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, service.ErrIdempotencyConflict):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, wallet)
}

func (s *Server) handleAdminManualRechargeCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	var req service.AdminManualRechargeInput
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	wallet, replayed, err := s.service.ApplyAdminManualRecharge(r.Context(), service.AdminActor{
		UserID: user.ID,
		Email:  user.Email,
	}, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAmount), errors.Is(err, service.ErrInvalidRequestID):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, service.ErrIdempotencyConflict):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, wallet)
}

func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	filter := service.AuditLogFilter{
		Action:      strings.TrimSpace(r.URL.Query().Get("action")),
		TargetType:  strings.TrimSpace(r.URL.Query().Get("target_type")),
		TargetID:    strings.TrimSpace(r.URL.Query().Get("target_id")),
		ActorUserID: strings.TrimSpace(r.URL.Query().Get("actor_user_id")),
		RiskLevel:   strings.TrimSpace(r.URL.Query().Get("risk_level")),
		SinceUnix:   parseUnixSeconds(r.URL.Query().Get("since_unix")),
		UntilUnix:   parseUnixSeconds(r.URL.Query().Get("until_unix")),
		Limit:       parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:      parseNonNegativeInt(r.URL.Query().Get("offset")),
	}
	items, err := s.service.ListAuditLogs(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "csv") {
		writeAdminAuditLogsCSV(w, items)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func writeAdminAuditLogsCSV(w http.ResponseWriter, items []service.AdminAuditLog) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="admin-audit-logs.csv"`)
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"created_unix", "actor_user_id", "actor_email", "action", "target_type", "target_id", "risk_level", "detail"})
	for _, item := range items {
		_ = writer.Write([]string{
			strconv.FormatInt(item.CreatedUnix, 10),
			sanitizeCSVCell(item.ActorUserID),
			sanitizeCSVCell(item.ActorEmail),
			sanitizeCSVCell(item.Action),
			sanitizeCSVCell(item.TargetType),
			sanitizeCSVCell(item.TargetID),
			sanitizeCSVCell(item.RiskLevel),
			sanitizeCSVCell(item.Detail),
		})
	}
	writer.Flush()
}

func sanitizeCSVCell(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch value[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func (s *Server) handleAdminRefundRequests(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListRefundRequests(r.Context(), service.RefundRequestFilter{
		UserID:      strings.TrimSpace(r.URL.Query().Get("user_id")),
		UserKeyword: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("user_keyword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		OrderID:     strings.TrimSpace(r.URL.Query().Get("order_id")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:       parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:      parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
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
	items, err := s.service.ListInfringementReports(r.Context(), service.InfringementReportFilter{
		UserID: user.ID,
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit")),
		Offset: parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminInfringementReports(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListInfringementReports(r.Context(), service.InfringementReportFilter{
		UserID:      strings.TrimSpace(r.URL.Query().Get("user_id")),
		UserKeyword: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("user_keyword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		ReviewedBy:  strings.TrimSpace(r.URL.Query().Get("reviewed_by")),
		Limit:       parsePositiveInt(r.URL.Query().Get("limit")),
		Offset:      parseNonNegativeInt(r.URL.Query().Get("offset")),
	})
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
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminDataRetentionPoliciesPut(w http.ResponseWriter, r *http.Request) {
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var items []service.DataRetentionPolicy
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveDataRetentionPoliciesWithRevision(r.Context(), expectedRevision, items); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusInternalServerError, "")
		http.Error(w, message, status)
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
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminSystemNotices(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListSystemNotices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminSystemNoticesPut(w http.ResponseWriter, r *http.Request) {
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var items []service.SystemNotice
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveSystemNoticesWithRevision(r.Context(), expectedRevision, items); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusInternalServerError, "")
		http.Error(w, message, status)
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
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminRiskRules(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListRiskRules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminRiskRulesPut(w http.ResponseWriter, r *http.Request) {
	expectedRevision, ok := requireExpectedRevision(w, r)
	if !ok {
		return
	}
	var items []service.RiskRule
	if err := decodeJSONBody(w, r, &items); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.service.SaveRiskRulesWithRevision(r.Context(), expectedRevision, items); err != nil {
		status, message := statusAndMessageFromError(err, http.StatusInternalServerError, "")
		http.Error(w, message, status)
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
	if err := writeJSONWithRevision(w, http.StatusOK, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
		operator, err := s.service.GetAdminOperator(r.Context(), user.ID, user.Email)
		if err != nil {
			if errors.Is(err, service.ErrAdminAccessDenied) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !operator.Active {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), adminOperatorKey{}, operator)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type authUserKey struct{}
type adminOperatorKey struct{}
type adminSessionResponse struct {
	User     AuthUser              `json:"user"`
	Operator service.AdminOperator `json:"operator"`
}

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

func adminOperatorFromContext(ctx context.Context) (service.AdminOperator, error) {
	operator, ok := ctx.Value(adminOperatorKey{}).(service.AdminOperator)
	if !ok || strings.TrimSpace(operator.Email) == "" {
		return service.AdminOperator{}, errors.New("missing admin operator")
	}
	return operator, nil
}

func requireAdminOperator(w http.ResponseWriter, r *http.Request) (service.AdminOperator, bool) {
	operator, err := adminOperatorFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return service.AdminOperator{}, false
	}
	return operator, true
}

func (s *Server) adminCapabilityMiddleware(capability string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operator, err := adminOperatorFromContext(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if !operator.HasCapability(capability) {
			http.Error(w, service.ErrAdminCapabilityDenied.Error(), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticateRequest(w http.ResponseWriter, r *http.Request, allowAdminCookie bool, next http.Handler) {
	if s.verifier == nil {
		http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
		return
	}
	token, usedAdminCookie, err := requestAccessToken(r, allowAdminCookie)
	if err != nil {
		if usedAdminCookie {
			s.clearAdminSessionCookie(w, r)
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if usedAdminCookie {
		if err := validateAdminSessionOrigin(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}
	user, err := s.verifier.Verify(r.Context(), token)
	if err != nil {
		if usedAdminCookie {
			s.clearAdminSessionCookie(w, r)
			http.Error(w, "invalid administrator session", http.StatusUnauthorized)
			return
		}
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
		return
	}
	s.mirrorUserIdentity(r.Context(), user.ID, user.Email, "")
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
}

func (s *Server) authenticateAdminRequest(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if s.verifier == nil {
		http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
		return
	}
	token, err := requestAdminSessionToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err := validateAdminSessionOrigin(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	user, err := s.verifier.Verify(r.Context(), token)
	if err != nil {
		s.clearAdminSessionCookie(w, r)
		http.Error(w, "invalid administrator session", http.StatusUnauthorized)
		return
	}
	s.mirrorUserIdentity(r.Context(), user.ID, user.Email, "")
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
}

func requestAccessToken(r *http.Request, allowAdminCookie bool) (token string, usedAdminCookie bool, err error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header != "" {
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return "", false, errors.New("missing bearer token")
		}
		token = strings.TrimSpace(header[7:])
		if token == "" {
			return "", false, errors.New("missing bearer token")
		}
		return token, false, nil
	}
	if !allowAdminCookie {
		return "", false, errors.New("missing bearer token")
	}
	cookie, cookieErr := r.Cookie(adminSessionCookieName)
	if cookieErr != nil {
		return "", false, errors.New("missing administrator session")
	}
	token = strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", true, errors.New("missing administrator session")
	}
	return token, true, nil
}

func requestAdminSessionToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return "", errors.New("missing administrator session")
	}
	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", errors.New("missing administrator session")
	}
	return token, nil
}

func validateAdminSessionOrigin(r *http.Request) error {
	if r == nil || !requestRequiresSameOriginCheck(r) {
		return nil
	}
	expectedHost := strings.TrimSpace(r.Host)
	if expectedHost == "" {
		return errors.New("administrator session origin validation failed")
	}
	expectedScheme := "http"
	if requestUsesHTTPS(r) {
		expectedScheme = "https"
	}
	source := strings.TrimSpace(r.Header.Get("Origin"))
	if source == "" {
		source = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if source == "" {
		return errors.New("missing origin for administrator session")
	}
	parsed, err := url.Parse(source)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return errors.New("invalid origin for administrator session")
	}
	if !strings.EqualFold(parsed.Host, expectedHost) || !strings.EqualFold(parsed.Scheme, expectedScheme) {
		return errors.New("origin mismatch for administrator session")
	}
	return nil
}

func requestRequiresSameOriginCheck(r *http.Request) bool {
	switch strings.ToUpper(strings.TrimSpace(r.Method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
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

func writeJSONWithRevision(w http.ResponseWriter, status int, payload any) error {
	revision, err := revisiontoken.ForPayload(payload)
	if err != nil {
		return err
	}
	return writeJSONWithCustomRevision(w, status, payload, revision)
}

func writeJSONWithCustomRevision(w http.ResponseWriter, status int, payload any, revision string) error {
	w.Header().Set("ETag", revision)
	w.Header().Set("X-Resource-Version", revision)
	writeJSON(w, status, payload)
	return nil
}

func requestExpectedRevision(r *http.Request) string {
	expected := strings.TrimSpace(r.Header.Get("If-Match"))
	if expected == "" {
		expected = strings.TrimSpace(r.Header.Get("X-Resource-Version"))
	}
	return expected
}

func requireExpectedRevision(w http.ResponseWriter, r *http.Request) (string, bool) {
	expected := requestExpectedRevision(r)
	if expected != "" {
		return expected, true
	}
	http.Error(w, "missing configuration revision, please reload before saving", http.StatusPreconditionRequired)
	return "", false
}

func (s *Server) servicePaymentProvider() payments.Provider {
	return s.service.PaymentProvider()
}

func (s *Server) mirrorUserIdentity(ctx context.Context, userID, email, username string) {
	if s == nil || s.service == nil {
		return
	}
	_ = s.service.UpsertUserIdentity(ctx, service.UserIdentity{
		UserID:   userID,
		Username: strings.TrimSpace(username),
		Email:    email,
	})
}

func (s *Server) lookupMirroredUsername(ctx context.Context, userID string) string {
	if s == nil || s.service == nil || strings.TrimSpace(userID) == "" {
		return ""
	}
	items, err := s.service.ListUsers(ctx, service.UserSummaryFilter{UserID: strings.TrimSpace(userID), Limit: 1})
	if err != nil || len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].Username)
}

func (s *Server) setAdminSessionCookie(w http.ResponseWriter, r *http.Request, session platformapi.Session) {
	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    strings.TrimSpace(session.AccessToken),
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   requestUsesHTTPS(r),
	}
	if session.ExpiresAt > 0 {
		expiry := time.Unix(session.ExpiresAt, 0)
		cookie.Expires = expiry
		cookie.MaxAge = max(int(time.Until(expiry).Seconds()), 0)
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   requestUsesHTTPS(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func requestUsesHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func toServiceAgreementDocuments(docs []platformapi.AgreementDocument) []service.AgreementDocument {
	items := make([]service.AgreementDocument, 0, len(docs))
	for _, doc := range docs {
		items = append(items, service.AgreementDocument{
			Key:               doc.Key,
			Version:           doc.Version,
			Title:             doc.Title,
			Content:           doc.Content,
			URL:               doc.URL,
			EffectiveFromUnix: doc.EffectiveFromUnix,
		})
	}
	return items
}

func statusAndMessageFromError(err error, fallbackStatus int, fallbackMessage string) (int, string) {
	if errors.Is(err, service.ErrRevisionConflict) {
		return http.StatusPreconditionFailed, "configuration changed, please reload and retry the save"
	}
	var apiErr *platformapi.APIError
	if errors.As(err, &apiErr) {
		message := localizeUserFacingErrorMessage(strings.TrimSpace(apiErr.Message))
		if message == "" {
			message = localizeUserFacingErrorMessage(err.Error())
		}
		if apiErr.StatusCode > 0 {
			return apiErr.StatusCode, message
		}
		return fallbackStatus, message
	}
	if strings.TrimSpace(fallbackMessage) != "" {
		return fallbackStatus, localizeUserFacingErrorMessage(fallbackMessage)
	}
	return fallbackStatus, localizeUserFacingErrorMessage(err.Error())
}

func localizeUserFacingErrorMessage(message string) string {
	return platformapi.NormalizeUserFacingErrorMessage(message)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	return json.NewDecoder(r.Body).Decode(target)
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	return json.NewDecoder(r.Body).Decode(target)
}

func parsePositiveInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func parseNonNegativeInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func parseUnixSeconds(raw string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
