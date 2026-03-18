package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"openclaw/platform/internal/payments"
	"openclaw/platform/internal/revisiontoken"
)

type UserSummary struct {
	UserID         string `json:"user_id"`
	UserNo         int64  `json:"user_no,omitempty"`
	Username       string `json:"username,omitempty"`
	Email          string `json:"email,omitempty"`
	CreatedUnix    int64  `json:"created_unix,omitempty"`
	LastSeenUnix   int64  `json:"last_seen_unix,omitempty"`
	BalanceFen     int64  `json:"balance_fen"`
	Currency       string `json:"currency"`
	UpdatedUnix    int64  `json:"updated_unix"`
	OrderCount     int    `json:"order_count,omitempty"`
	RefundCount    int    `json:"refund_count,omitempty"`
	LastOrderUnix  int64  `json:"last_order_unix,omitempty"`
	LastRefundUnix int64  `json:"last_refund_unix,omitempty"`
}

type UserIdentity struct {
	UserID       string `json:"user_id"`
	UserNo       int64  `json:"user_no,omitempty"`
	Username     string `json:"username,omitempty"`
	Email        string `json:"email,omitempty"`
	CreatedUnix  int64  `json:"created_unix,omitempty"`
	UpdatedUnix  int64  `json:"updated_unix,omitempty"`
	LastSeenUnix int64  `json:"last_seen_unix,omitempty"`
}

type AdminActor struct {
	UserID string `json:"user_id,omitempty"`
	Email  string `json:"email,omitempty"`
}

type AdminWalletAdjustmentInput struct {
	UserID      string `json:"user_id"`
	AmountFen   int64  `json:"amount_fen"`
	Description string `json:"description,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
}

type AdminManualRechargeInput struct {
	UserID      string `json:"user_id"`
	AmountFen   int64  `json:"amount_fen"`
	Description string `json:"description,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
}

type AgreementAcceptance struct {
	UserID          string `json:"user_id"`
	AgreementKey    string `json:"agreement_key"`
	Version         string `json:"version"`
	AcceptedUnix    int64  `json:"accepted_unix"`
	ClientVersion   string `json:"client_version,omitempty"`
	RemoteAddr      string `json:"remote_addr,omitempty"`
	DeviceSummary   string `json:"device_summary,omitempty"`
	ContentChecksum string `json:"content_checksum,omitempty"`
}

type AgreementAcceptanceSource struct {
	ClientVersion string
	RemoteAddr    string
	DeviceSummary string
}

type ChatUsageRecord struct {
	ID                string   `json:"id"`
	UserID            string   `json:"user_id"`
	ModelID           string   `json:"model_id"`
	PricingVersion    string   `json:"pricing_version,omitempty"`
	PromptTokens      int      `json:"prompt_tokens,omitempty"`
	CompletionTokens  int      `json:"completion_tokens,omitempty"`
	ChargedFen        int64    `json:"charged_fen"`
	FallbackApplied   bool     `json:"fallback_applied,omitempty"`
	RequestKind       string   `json:"request_kind,omitempty"`
	CreatedUnix       int64    `json:"created_unix"`
	AgreementVersions []string `json:"agreement_versions,omitempty"`
}

type AdminAuditLog struct {
	ID          string `json:"id"`
	ActorUserID string `json:"actor_user_id,omitempty"`
	ActorEmail  string `json:"actor_email,omitempty"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	RiskLevel   string `json:"risk_level,omitempty"`
	Detail      string `json:"detail,omitempty"`
	CreatedUnix int64  `json:"created_unix"`
}

type AuditLogFilter struct {
	Action      string
	TargetType  string
	TargetID    string
	ActorUserID string
	RiskLevel   string
	SinceUnix   int64
	UntilUnix   int64
	Limit       int
	Offset      int
}

type UserSummaryFilter struct {
	UserID  string
	Email   string
	Keyword string
	Limit   int
	Offset  int
}

type RechargeOrderFilter struct {
	UserID      string
	UserKeyword string
	Status      string
	Provider    string
	Limit       int
	Offset      int
}

type RefundRequest struct {
	ID               string `json:"id"`
	UserID           string `json:"user_id"`
	UserNo           int64  `json:"user_no,omitempty"`
	Username         string `json:"username,omitempty"`
	OrderID          string `json:"order_id"`
	AmountFen        int64  `json:"amount_fen"`
	Reason           string `json:"reason,omitempty"`
	Status           string `json:"status"`
	ReviewNote       string `json:"review_note,omitempty"`
	ReviewedBy       string `json:"reviewed_by,omitempty"`
	RefundProvider   string `json:"refund_provider,omitempty"`
	ExternalRefundID string `json:"external_refund_id,omitempty"`
	ExternalStatus   string `json:"external_status,omitempty"`
	FailureReason    string `json:"failure_reason,omitempty"`
	CreatedUnix      int64  `json:"created_unix"`
	UpdatedUnix      int64  `json:"updated_unix"`
	SettledUnix      int64  `json:"settled_unix,omitempty"`
}

type RefundDecisionInput struct {
	Status           string `json:"status"`
	ReviewNote       string `json:"review_note,omitempty"`
	ReviewedBy       string `json:"reviewed_by,omitempty"`
	RefundProvider   string `json:"refund_provider,omitempty"`
	ExternalRefundID string `json:"external_refund_id,omitempty"`
	ExternalStatus   string `json:"external_status,omitempty"`
	FailureReason    string `json:"failure_reason,omitempty"`
}

type RefundRequestFilter struct {
	UserID      string
	UserKeyword string
	OrderID     string
	Status      string
	Limit       int
	Offset      int
}

type WalletAdjustmentFilter struct {
	UserID        string
	UserKeyword   string
	Kind          string
	ReferenceType string
	Limit         int
	Offset        int
}

type InfringementReport struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	UserNo       int64    `json:"user_no,omitempty"`
	Username     string   `json:"username,omitempty"`
	Subject      string   `json:"subject"`
	Description  string   `json:"description"`
	EvidenceURLs []string `json:"evidence_urls,omitempty"`
	Status       string   `json:"status"`
	Resolution   string   `json:"resolution,omitempty"`
	ReviewedBy   string   `json:"reviewed_by,omitempty"`
	CreatedUnix  int64    `json:"created_unix"`
	UpdatedUnix  int64    `json:"updated_unix"`
}

type InfringementUpdateInput struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution,omitempty"`
	ReviewedBy string `json:"reviewed_by,omitempty"`
}

type InfringementReportFilter struct {
	UserID      string
	UserKeyword string
	Status      string
	ReviewedBy  string
	Limit       int
	Offset      int
}

type DataRetentionPolicy struct {
	DataDomain    string `json:"data_domain"`
	RetentionDays int    `json:"retention_days"`
	PurgeMode     string `json:"purge_mode,omitempty"`
	Description   string `json:"description,omitempty"`
	Enabled       bool   `json:"enabled"`
	UpdatedUnix   int64  `json:"updated_unix,omitempty"`
}

type SystemNotice struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Severity    string `json:"severity,omitempty"`
	Enabled     bool   `json:"enabled"`
	UpdatedUnix int64  `json:"updated_unix,omitempty"`
}

type RiskRule struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	UpdatedUnix int64  `json:"updated_unix,omitempty"`
}

type OfficialAccessState struct {
	Enabled                  bool   `json:"enabled"`
	BalanceFen               int64  `json:"balance_fen"`
	Currency                 string `json:"currency,omitempty"`
	LowBalance               bool   `json:"low_balance"`
	ModelsConfigured         int    `json:"models_configured,omitempty"`
	MinimumReserveFen        int64  `json:"minimum_reserve_fen,omitempty"`
	MinimumRechargeAmountFen int64  `json:"minimum_recharge_amount_fen,omitempty"`
}

type governanceFilteredStore interface {
	ListUsersFiltered(ctx context.Context, filter UserSummaryFilter) ([]UserSummary, error)
	ListOrdersFiltered(ctx context.Context, filter RechargeOrderFilter) ([]RechargeOrder, error)
	ListWalletAdjustmentsFiltered(ctx context.Context, filter WalletAdjustmentFilter) ([]WalletTransaction, error)
	ListRefundRequestsFiltered(ctx context.Context, filter RefundRequestFilter) ([]RefundRequest, error)
	ListInfringementReportsFiltered(ctx context.Context, filter InfringementReportFilter) ([]InfringementReport, error)
}

type governanceRevisionStore interface {
	SaveDataRetentionPoliciesWithRevision(ctx context.Context, expectedRevision string, policies []DataRetentionPolicy) error
	SaveSystemNoticesWithRevision(ctx context.Context, expectedRevision string, notices []SystemNotice) error
	SaveRiskRulesWithRevision(ctx context.Context, expectedRevision string, rules []RiskRule) error
}

type adminWalletMutationSpec struct {
	defaultDescription string
	referenceType      string
	referencePrefix    string
	auditAction        string
	requirePositive    bool
}

func (s *Service) ListUsers(ctx context.Context, filter UserSummaryFilter) ([]UserSummary, error) {
	if store, ok := s.store.(governanceFilteredStore); ok {
		return store.ListUsersFiltered(ctx, filter)
	}
	items, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return filterUserSummaries(items, filter), nil
}

func (s *Service) UpsertUserIdentity(ctx context.Context, identity UserIdentity) error {
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.Email = strings.TrimSpace(identity.Email)
	now := s.now().Unix()
	if identity.UserID == "" {
		return nil
	}
	if identity.CreatedUnix == 0 {
		identity.CreatedUnix = now
	}
	if identity.UpdatedUnix == 0 {
		identity.UpdatedUnix = now
	}
	if identity.LastSeenUnix == 0 {
		identity.LastSeenUnix = now
	}
	return s.store.UpsertUserIdentity(ctx, identity)
}

func (s *Service) ApplyAdminWalletAdjustment(ctx context.Context, actor AdminActor, input AdminWalletAdjustmentInput) (WalletSummary, bool, error) {
	return s.applyAdminWalletMutation(ctx, actor, input.UserID, input.AmountFen, input.Description, input.RequestID, adminWalletMutationSpec{
		defaultDescription: "admin wallet adjustment",
		referenceType:      "admin_adjustment",
		referencePrefix:    "admin_adj",
		auditAction:        "admin.wallet_adjustment.created",
	})
}

func (s *Service) ApplyAdminManualRecharge(ctx context.Context, actor AdminActor, input AdminManualRechargeInput) (WalletSummary, bool, error) {
	return s.applyAdminWalletMutation(ctx, actor, input.UserID, input.AmountFen, input.Description, input.RequestID, adminWalletMutationSpec{
		defaultDescription: "admin manual recharge",
		referenceType:      "admin_manual_recharge",
		referencePrefix:    "admin_recharge",
		auditAction:        "admin.manual_recharge.created",
		requirePositive:    true,
	})
}

func (s *Service) applyAdminWalletMutation(
	ctx context.Context,
	actor AdminActor,
	userID string,
	amountFen int64,
	description string,
	requestID string,
	spec adminWalletMutationSpec,
) (WalletSummary, bool, error) {
	userID = strings.TrimSpace(userID)
	description = strings.TrimSpace(description)
	requestID = strings.TrimSpace(requestID)
	actor.UserID = strings.TrimSpace(actor.UserID)
	actor.Email = strings.TrimSpace(actor.Email)
	if userID == "" || amountFen == 0 {
		return WalletSummary{}, false, ErrInvalidAmount
	}
	if spec.requirePositive && amountFen < 0 {
		return WalletSummary{}, false, ErrInvalidAmount
	}
	if requestID == "" {
		return WalletSummary{}, false, ErrInvalidRequestID
	}
	if err := s.ensureAdminWalletTargetUserExists(ctx, userID); err != nil {
		return WalletSummary{}, false, err
	}
	if description == "" {
		description = spec.defaultDescription
	}
	now := s.now()
	wallet, replayed, err := s.store.ApplyAdminWalletMutation(ctx, WalletTransaction{
		ID:            fmt.Sprintf("tx_%d", now.UnixNano()),
		UserID:        userID,
		Kind:          walletAdjustmentKind(amountFen),
		AmountFen:     amountFen,
		Description:   description,
		ReferenceType: spec.referenceType,
		ReferenceID:   requestID,
		CreatedUnix:   now.Unix(),
	}, AdminAuditLog{
		ID:          fmt.Sprintf("%s_audit_%d", spec.referencePrefix, now.UnixNano()),
		ActorUserID: actor.UserID,
		ActorEmail:  actor.Email,
		Action:      spec.auditAction,
		TargetType:  "wallet_account",
		TargetID:    userID,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("amount_fen=%d description=%s request_id=%s", amountFen, description, requestID),
		CreatedUnix: now.Unix(),
	})
	if err != nil {
		return WalletSummary{}, false, err
	}
	return wallet, replayed, nil
}

func (s *Service) ensureAdminWalletTargetUserExists(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("%w: empty target user id", ErrTargetUserNotFound)
	}
	items, err := s.ListUsers(ctx, UserSummaryFilter{
		UserID: userID,
		Limit:  1,
	})
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: %s", ErrTargetUserNotFound, userID)
	}
	return nil
}

func (s *Service) ListOrders(ctx context.Context, filter RechargeOrderFilter) ([]RechargeOrder, error) {
	if store, ok := s.store.(governanceFilteredStore); ok {
		return store.ListOrdersFiltered(ctx, filter)
	}
	items, err := s.store.ListOrders(ctx)
	if err != nil {
		return nil, err
	}
	return filterRechargeOrders(items, filter), nil
}

func (s *Service) ListWalletAdjustments(ctx context.Context, filter WalletAdjustmentFilter) ([]WalletTransaction, error) {
	if store, ok := s.store.(governanceFilteredStore); ok {
		return store.ListWalletAdjustmentsFiltered(ctx, filter)
	}
	items, err := s.store.ListWalletAdjustments(ctx)
	if err != nil {
		return nil, err
	}
	return filterWalletAdjustments(items, filter), nil
}

func (s *Service) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AdminAuditLog, error) {
	return s.store.ListAuditLogs(ctx, filter)
}

func (s *Service) ListRefundRequests(ctx context.Context, filter RefundRequestFilter) ([]RefundRequest, error) {
	if store, ok := s.store.(governanceFilteredStore); ok {
		return store.ListRefundRequestsFiltered(ctx, filter)
	}
	items, err := s.store.ListRefundRequests(ctx, strings.TrimSpace(filter.UserID))
	if err != nil {
		return nil, err
	}
	return filterRefundRequests(items, filter), nil
}

func (s *Service) CreateRefundRequest(ctx context.Context, userID string, amountFen int64, orderID, reason string) (RefundRequest, error) {
	order, err := s.store.GetOrder(ctx, userID, orderID)
	if err != nil {
		return RefundRequest{}, err
	}
	if order.Status != "paid" {
		return RefundRequest{}, fmt.Errorf("%w: order %s is not paid", ErrRefundNotAllowed, orderID)
	}
	if amountFen <= 0 {
		return RefundRequest{}, ErrInvalidAmount
	}
	refundableFen := order.AmountFen - order.RefundedFen
	if refundableFen <= 0 || amountFen > refundableFen {
		return RefundRequest{}, fmt.Errorf("%w: order %s only has %d fen refundable", ErrRefundNotAllowed, orderID, refundableFen)
	}
	wallet, err := s.store.GetWallet(ctx, userID)
	if err != nil {
		return RefundRequest{}, err
	}
	if wallet.BalanceFen < amountFen {
		return RefundRequest{}, fmt.Errorf("%w: wallet balance %d fen is lower than requested refund %d fen", ErrRefundNotAllowed, wallet.BalanceFen, amountFen)
	}
	request := RefundRequest{
		ID:          fmt.Sprintf("refund_%d", s.now().UnixNano()),
		UserID:      userID,
		OrderID:     orderID,
		AmountFen:   amountFen,
		Reason:      strings.TrimSpace(reason),
		Status:      "pending",
		CreatedUnix: s.now().Unix(),
		UpdatedUnix: s.now().Unix(),
	}
	if err := s.store.CreateRefundRequest(ctx, request); err != nil {
		return RefundRequest{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: userID,
		Action:      "refund_request.created",
		TargetType:  "refund_request",
		TargetID:    request.ID,
		RiskLevel:   "medium",
		Detail:      fmt.Sprintf("order=%s amount_fen=%d", orderID, amountFen),
		CreatedUnix: s.now().Unix(),
	})
	return request, nil
}

func (s *Service) ReviewRefundRequest(ctx context.Context, requestID string, input RefundDecisionInput) (RefundRequest, error) {
	request, err := s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	if err != nil {
		return RefundRequest{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: request.ReviewedBy,
		Action:      "refund_request.reviewed",
		TargetType:  "refund_request",
		TargetID:    request.ID,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("status=%s order=%s amount_fen=%d", request.Status, request.OrderID, request.AmountFen),
		CreatedUnix: s.now().Unix(),
	})
	return request, nil
}

func (s *Service) ApproveRefundRequest(ctx context.Context, requestID string, input RefundDecisionInput) (RefundRequest, error) {
	request, err := s.store.GetRefundRequest(ctx, requestID)
	if err != nil {
		return RefundRequest{}, err
	}
	order, err := s.store.FindOrderByID(ctx, request.OrderID)
	if err != nil {
		return RefundRequest{}, err
	}
	input.ReviewedBy = strings.TrimSpace(input.ReviewedBy)
	input.RefundProvider = firstNonEmpty(strings.TrimSpace(input.RefundProvider), order.Provider, s.payment.Name())

	if s.payment == nil {
		s.payment = payments.ManualProvider{}
	}
	refundResult, refundErr := s.payment.Refund(ctx, payments.RefundInput{
		OrderID:         order.ID,
		ExternalOrderID: order.ExternalID,
		AmountFen:       request.AmountFen,
		Reason:          firstNonEmpty(strings.TrimSpace(request.Reason), strings.TrimSpace(input.ReviewNote)),
	})
	switch {
	case refundErr == nil && refundResult.Succeeded:
		input.Status = "refunded"
		input.ExternalRefundID = refundResult.ExternalRefundID
		input.ExternalStatus = refundResult.ProviderStatus
		request, err = s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	case refundErr == nil && refundResult.Pending:
		input.Status = "approved_pending_payout"
		input.ExternalRefundID = refundResult.ExternalRefundID
		input.ExternalStatus = refundResult.ProviderStatus
		request, err = s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	case errors.Is(refundErr, payments.ErrOperationNotSupported):
		input.Status = "approved_pending_payout"
		input.ExternalStatus = "manual_payout_required"
		input.FailureReason = refundErr.Error()
		request, err = s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	default:
		input.Status = "refund_failed"
		if refundErr != nil {
			input.FailureReason = refundErr.Error()
		} else {
			input.FailureReason = refundResult.Message
		}
		input.ExternalRefundID = refundResult.ExternalRefundID
		input.ExternalStatus = refundResult.ProviderStatus
		request, err = s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	}
	if err != nil {
		return RefundRequest{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: input.ReviewedBy,
		Action:      "refund_request.approved",
		TargetType:  "refund_request",
		TargetID:    request.ID,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("status=%s external_status=%s", request.Status, request.ExternalStatus),
		CreatedUnix: s.now().Unix(),
	})
	return request, nil
}

func (s *Service) MarkRefundSettled(ctx context.Context, requestID string, input RefundDecisionInput) (RefundRequest, error) {
	input.Status = "refunded"
	request, err := s.store.ApplyRefundDecision(ctx, requestID, input, s.now().Unix())
	if err != nil {
		return RefundRequest{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: input.ReviewedBy,
		Action:      "refund_request.settled",
		TargetType:  "refund_request",
		TargetID:    request.ID,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("external_refund_id=%s", request.ExternalRefundID),
		CreatedUnix: s.now().Unix(),
	})
	return request, nil
}

func (s *Service) ReconcileRechargeOrder(ctx context.Context, orderID string) (RechargeOrder, bool, error) {
	order, err := s.store.FindOrderByID(ctx, orderID)
	if err != nil {
		return RechargeOrder{}, false, err
	}
	if s.payment == nil {
		s.payment = payments.ManualProvider{}
	}
	status, err := s.payment.QueryOrder(ctx, payments.QueryOrderInput{
		OrderID:         order.ID,
		ExternalOrderID: order.ExternalID,
	})
	if err != nil {
		return RechargeOrder{}, false, err
	}
	order.ProviderStatus = status.ProviderStatus
	order.LastCheckedUnix = status.LastCheckedUnix
	changed := false
	switch {
	case status.Paid && order.Status == "pending":
		changed, err = s.HandleSuccessfulRecharge(ctx, order.ID, order.Provider, firstNonEmpty(status.ExternalOrderID, order.ExternalID))
		if err != nil {
			return RechargeOrder{}, false, err
		}
		order, err = s.store.FindOrderByID(ctx, order.ID)
		if err != nil {
			return RechargeOrder{}, false, err
		}
		order.ProviderStatus = status.ProviderStatus
		order.LastCheckedUnix = status.LastCheckedUnix
		if err := s.store.SaveOrder(ctx, order); err != nil {
			return RechargeOrder{}, false, err
		}
	case status.Refunded && order.Status != "refunded":
		order, changed, err = s.reconcileExternallyRefundedOrder(ctx, order, status)
		if err != nil {
			return RechargeOrder{}, false, err
		}
	case order.Status == "paid" && !status.Refunded:
		order.ProviderStatus = status.ProviderStatus
		order.LastCheckedUnix = status.LastCheckedUnix
		order.ExternalID = firstNonEmpty(status.ExternalOrderID, order.ExternalID)
		order.UpdatedUnix = s.now().Unix()
		if err := s.store.SaveOrder(ctx, order); err != nil {
			return RechargeOrder{}, false, err
		}
	default:
		order.Status = status.Status
		order.UpdatedUnix = s.now().Unix()
		if err := s.store.SaveOrder(ctx, order); err != nil {
			return RechargeOrder{}, false, err
		}
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		Action:      "wallet.order.reconciled",
		TargetType:  "recharge_order",
		TargetID:    order.ID,
		RiskLevel:   "medium",
		Detail:      fmt.Sprintf("provider_status=%s changed=%t", status.ProviderStatus, changed),
		CreatedUnix: s.now().Unix(),
	})
	return order, changed, nil
}

func (s *Service) reconcileExternallyRefundedOrder(
	ctx context.Context,
	order RechargeOrder,
	status payments.OrderStatusResult,
) (RechargeOrder, bool, error) {
	refundableFen := order.AmountFen - order.RefundedFen
	refundProvider := strings.TrimSpace(order.Provider)
	if refundProvider == "" && s.payment != nil {
		refundProvider = strings.TrimSpace(s.payment.Name())
	}
	if refundableFen > 0 {
		requestID := fmt.Sprintf("refund_reconcile_%s", order.ID)
		request, err := s.store.GetRefundRequest(ctx, requestID)
		if err != nil {
			request = RefundRequest{
				ID:             requestID,
				UserID:         order.UserID,
				OrderID:        order.ID,
				AmountFen:      refundableFen,
				Reason:         "provider reconciliation marked order refunded",
				Status:         "pending",
				RefundProvider: order.Provider,
				CreatedUnix:    s.now().Unix(),
				UpdatedUnix:    s.now().Unix(),
			}
			if err := s.store.CreateRefundRequest(ctx, request); err != nil {
				return RechargeOrder{}, false, err
			}
		}
		if request.Status != "refunded" {
			if _, err := s.MarkRefundSettled(ctx, requestID, RefundDecisionInput{
				ReviewedBy:     "system:reconcile",
				ReviewNote:     "provider reconciliation marked order refunded",
				RefundProvider: refundProvider,
				ExternalStatus: status.ProviderStatus,
			}); err != nil {
				return RechargeOrder{}, false, err
			}
		}
	}
	refreshed, err := s.store.FindOrderByID(ctx, order.ID)
	if err != nil {
		return RechargeOrder{}, false, err
	}
	refreshed.Status = "refunded"
	refreshed.RefundedFen = maxInt64(refreshed.RefundedFen, refreshed.AmountFen)
	refreshed.ProviderStatus = status.ProviderStatus
	refreshed.LastCheckedUnix = status.LastCheckedUnix
	refreshed.ExternalID = firstNonEmpty(status.ExternalOrderID, refreshed.ExternalID)
	refreshed.PaidUnix = maxInt64(refreshed.PaidUnix, s.now().Unix())
	refreshed.UpdatedUnix = s.now().Unix()
	if err := s.store.SaveOrder(ctx, refreshed); err != nil {
		return RechargeOrder{}, false, err
	}
	return refreshed, true, nil
}

func (s *Service) ReconcilePendingRechargeOrders(ctx context.Context) ([]RechargeOrder, error) {
	orders, err := s.store.ListOrders(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]RechargeOrder, 0)
	for _, order := range orders {
		if order.Status != "pending" {
			continue
		}
		reconciled, _, err := s.ReconcileRechargeOrder(ctx, order.ID)
		if err != nil {
			continue
		}
		items = append(items, reconciled)
	}
	return items, nil
}

func (s *Service) CreateInfringementReport(ctx context.Context, report InfringementReport) (InfringementReport, error) {
	report.ID = fmt.Sprintf("ipr_%d", s.now().UnixNano())
	report.Subject = strings.TrimSpace(report.Subject)
	report.Description = strings.TrimSpace(report.Description)
	evidenceURLs, err := normalizeEvidenceURLs(report.EvidenceURLs)
	if err != nil {
		return InfringementReport{}, err
	}
	report.EvidenceURLs = evidenceURLs
	report.Status = "pending"
	report.CreatedUnix = s.now().Unix()
	report.UpdatedUnix = report.CreatedUnix
	if report.Subject == "" || report.Description == "" {
		return InfringementReport{}, fmt.Errorf("subject and description are required")
	}
	if err := s.store.CreateInfringementReport(ctx, report); err != nil {
		return InfringementReport{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: report.UserID,
		Action:      "infringement_report.created",
		TargetType:  "infringement_report",
		TargetID:    report.ID,
		RiskLevel:   "high",
		Detail:      report.Subject,
		CreatedUnix: s.now().Unix(),
	})
	return report, nil
}

func normalizeEvidenceURLs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed == nil || parsed.Host == "" {
			return nil, fmt.Errorf("invalid evidence url: %s", raw)
		}
		scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("invalid evidence url scheme: %s", raw)
		}
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		items = append(items, normalized)
	}
	return items, nil
}

func (s *Service) UpdateInfringementReport(ctx context.Context, reportID string, input InfringementUpdateInput) (InfringementReport, error) {
	report, err := s.store.GetInfringementReport(ctx, reportID)
	if err != nil {
		return InfringementReport{}, err
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != "" {
		report.Status = status
	}
	if note := strings.TrimSpace(input.Resolution); note != "" {
		report.Resolution = note
	}
	if reviewer := strings.TrimSpace(input.ReviewedBy); reviewer != "" {
		report.ReviewedBy = reviewer
	}
	report.UpdatedUnix = s.now().Unix()
	if err := s.store.SaveInfringementReport(ctx, report); err != nil {
		return InfringementReport{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ID:          fmt.Sprintf("audit_%d", s.now().UnixNano()),
		ActorUserID: report.ReviewedBy,
		Action:      "infringement_report.updated",
		TargetType:  "infringement_report",
		TargetID:    report.ID,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("status=%s", report.Status),
		CreatedUnix: s.now().Unix(),
	})
	return report, nil
}

func (s *Service) ListInfringementReports(ctx context.Context, filter InfringementReportFilter) ([]InfringementReport, error) {
	if store, ok := s.store.(governanceFilteredStore); ok {
		return store.ListInfringementReportsFiltered(ctx, filter)
	}
	items, err := s.store.ListInfringementReports(ctx, strings.TrimSpace(filter.UserID))
	if err != nil {
		return nil, err
	}
	return filterInfringementReports(items, filter), nil
}

func (s *Service) ListDataRetentionPolicies(ctx context.Context) ([]DataRetentionPolicy, error) {
	return s.store.ListDataRetentionPolicies(ctx)
}

func (s *Service) SaveDataRetentionPolicies(ctx context.Context, items []DataRetentionPolicy) error {
	return s.SaveDataRetentionPoliciesWithRevision(ctx, "", items)
}

func (s *Service) SaveDataRetentionPoliciesWithRevision(ctx context.Context, expectedRevision string, items []DataRetentionPolicy) error {
	now := s.now().Unix()
	for i := range items {
		items[i].DataDomain = strings.TrimSpace(items[i].DataDomain)
		if items[i].UpdatedUnix == 0 {
			items[i].UpdatedUnix = now
		}
	}
	if store, ok := s.store.(governanceRevisionStore); ok {
		return store.SaveDataRetentionPoliciesWithRevision(ctx, expectedRevision, items)
	}
	if err := ensureExpectedRevision(expectedRevision, s.store.ListDataRetentionPolicies, ctx); err != nil {
		return err
	}
	return s.store.SaveDataRetentionPolicies(ctx, items)
}

func (s *Service) ListSystemNotices(ctx context.Context) ([]SystemNotice, error) {
	return s.store.ListSystemNotices(ctx)
}

func (s *Service) SaveSystemNotices(ctx context.Context, items []SystemNotice) error {
	return s.SaveSystemNoticesWithRevision(ctx, "", items)
}

func (s *Service) SaveSystemNoticesWithRevision(ctx context.Context, expectedRevision string, items []SystemNotice) error {
	now := s.now().Unix()
	for i := range items {
		items[i].ID = strings.TrimSpace(items[i].ID)
		if items[i].UpdatedUnix == 0 {
			items[i].UpdatedUnix = now
		}
	}
	if store, ok := s.store.(governanceRevisionStore); ok {
		return store.SaveSystemNoticesWithRevision(ctx, expectedRevision, items)
	}
	if err := ensureExpectedRevision(expectedRevision, s.store.ListSystemNotices, ctx); err != nil {
		return err
	}
	return s.store.SaveSystemNotices(ctx, items)
}

func (s *Service) ListRiskRules(ctx context.Context) ([]RiskRule, error) {
	return s.store.ListRiskRules(ctx)
}

func (s *Service) SaveRiskRules(ctx context.Context, items []RiskRule) error {
	return s.SaveRiskRulesWithRevision(ctx, "", items)
}

func (s *Service) SaveRiskRulesWithRevision(ctx context.Context, expectedRevision string, items []RiskRule) error {
	now := s.now().Unix()
	for i := range items {
		items[i].Key = strings.TrimSpace(items[i].Key)
		if items[i].UpdatedUnix == 0 {
			items[i].UpdatedUnix = now
		}
	}
	if store, ok := s.store.(governanceRevisionStore); ok {
		return store.SaveRiskRulesWithRevision(ctx, expectedRevision, items)
	}
	if err := ensureExpectedRevision(expectedRevision, s.store.ListRiskRules, ctx); err != nil {
		return err
	}
	return s.store.SaveRiskRules(ctx, items)
}

func ensureExpectedRevision[T any](expected string, listFn func(context.Context) ([]T, error), ctx context.Context) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	current, err := listFn(ctx)
	if err != nil {
		return err
	}
	revision, err := revisiontoken.ForPayload(current)
	if err != nil {
		return err
	}
	if revisiontoken.Matches(expected, revision) {
		return nil
	}
	return fmt.Errorf("%w", ErrRevisionConflict)
}

func (s *Service) GetOfficialAccessState(ctx context.Context, userID string) (OfficialAccessState, error) {
	wallet, err := s.store.GetWallet(ctx, userID)
	if err != nil {
		return OfficialAccessState{}, err
	}
	models := s.ListEnabledOfficialModels(ctx)
	state := OfficialAccessState{
		Enabled:                  len(models) > 0,
		BalanceFen:               wallet.BalanceFen,
		Currency:                 wallet.Currency,
		LowBalance:               wallet.BalanceFen > 0 && wallet.BalanceFen < 100,
		ModelsConfigured:         len(models),
		MinimumReserveFen:        minimumReserveFen(models),
		MinimumRechargeAmountFen: s.WalletSettings().MinRechargeAmountFen,
	}
	return state, nil
}

func minimumReserveFen(models []OfficialModel) int64 {
	var minimum int64
	for _, model := range models {
		if model.ReserveFen <= 0 {
			continue
		}
		if minimum == 0 || model.ReserveFen < minimum {
			minimum = model.ReserveFen
		}
	}
	return minimum
}

func (s *Service) appendAuditLog(ctx context.Context, entry AdminAuditLog) error {
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = fmt.Sprintf("audit_%d", s.now().UnixNano())
	}
	if entry.CreatedUnix == 0 {
		entry.CreatedUnix = s.now().Unix()
	}
	if s.store == nil {
		return nil
	}
	return s.store.AppendAuditLog(ctx, entry)
}

func (s *Service) RecordAdminAudit(ctx context.Context, entry AdminAuditLog) error {
	return s.appendAuditLog(ctx, entry)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func filterRechargeOrders(items []RechargeOrder, filter RechargeOrderFilter) []RechargeOrder {
	userID := strings.TrimSpace(filter.UserID)
	userKeyword := strings.TrimSpace(filter.UserKeyword)
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	provider := strings.ToLower(strings.TrimSpace(filter.Provider))
	filtered := make([]RechargeOrder, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if userKeyword != "" && !matchesUserIdentityKeyword(item.UserID, item.UserNo, item.Username, "", userKeyword) {
			continue
		}
		if status != "" && strings.ToLower(strings.TrimSpace(item.Status)) != status {
			continue
		}
		if provider != "" && strings.ToLower(strings.TrimSpace(item.Provider)) != provider {
			continue
		}
		filtered = append(filtered, item)
	}
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func filterUserSummaries(items []UserSummary, filter UserSummaryFilter) []UserSummary {
	userID := strings.TrimSpace(filter.UserID)
	email := strings.ToLower(strings.TrimSpace(filter.Email))
	keyword := strings.TrimSpace(filter.Keyword)
	filtered := make([]UserSummary, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if email != "" && strings.ToLower(strings.TrimSpace(item.Email)) != email {
			continue
		}
		if keyword != "" && !matchesUserSummaryKeyword(item, keyword) {
			continue
		}
		filtered = append(filtered, item)
	}
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func matchesUserSummaryKeyword(item UserSummary, keyword string) bool {
	return matchesUserIdentityKeyword(item.UserID, item.UserNo, item.Username, item.Email, keyword)
}

func matchesUserIdentityKeyword(userID string, userNo int64, username, email, keyword string) bool {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return true
	}
	if userID == keyword {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(email), keyword) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(username), keyword) {
		return true
	}
	return userNo > 0 && strconv.FormatInt(userNo, 10) == keyword
}

func filterRefundRequests(items []RefundRequest, filter RefundRequestFilter) []RefundRequest {
	userID := strings.TrimSpace(filter.UserID)
	userKeyword := strings.TrimSpace(filter.UserKeyword)
	orderID := strings.TrimSpace(filter.OrderID)
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	filtered := make([]RefundRequest, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if userKeyword != "" && !matchesUserIdentityKeyword(item.UserID, item.UserNo, item.Username, "", userKeyword) {
			continue
		}
		if orderID != "" && item.OrderID != orderID {
			continue
		}
		if status != "" && strings.ToLower(strings.TrimSpace(item.Status)) != status {
			continue
		}
		filtered = append(filtered, item)
	}
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func filterWalletAdjustments(items []WalletTransaction, filter WalletAdjustmentFilter) []WalletTransaction {
	userID := strings.TrimSpace(filter.UserID)
	userKeyword := strings.TrimSpace(filter.UserKeyword)
	kind := strings.ToLower(strings.TrimSpace(filter.Kind))
	referenceType := strings.ToLower(strings.TrimSpace(filter.ReferenceType))
	filtered := make([]WalletTransaction, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if userKeyword != "" && !matchesUserIdentityKeyword(item.UserID, item.UserNo, item.Username, "", userKeyword) {
			continue
		}
		if kind != "" && strings.ToLower(strings.TrimSpace(item.Kind)) != kind {
			continue
		}
		if referenceType != "" && strings.ToLower(strings.TrimSpace(item.ReferenceType)) != referenceType {
			continue
		}
		filtered = append(filtered, item)
	}
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func filterInfringementReports(items []InfringementReport, filter InfringementReportFilter) []InfringementReport {
	userID := strings.TrimSpace(filter.UserID)
	userKeyword := strings.TrimSpace(filter.UserKeyword)
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	reviewedBy := strings.TrimSpace(filter.ReviewedBy)
	filtered := make([]InfringementReport, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if userKeyword != "" && !matchesUserIdentityKeyword(item.UserID, item.UserNo, item.Username, "", userKeyword) {
			continue
		}
		if status != "" && strings.ToLower(strings.TrimSpace(item.Status)) != status {
			continue
		}
		if reviewedBy != "" && item.ReviewedBy != reviewedBy {
			continue
		}
		filtered = append(filtered, item)
	}
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func applyWindow[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	items = items[offset:]
	if limit <= 0 || limit >= len(items) {
		return append([]T(nil), items...)
	}
	return append([]T(nil), items[:limit]...)
}

func walletAdjustmentKind(amountFen int64) string {
	if amountFen < 0 {
		return "debit"
	}
	return "credit"
}

func selectActivePricingRule(now time.Time, modelID string, rules []PricingRule) (PricingRule, bool) {
	modelID = strings.TrimSpace(modelID)
	var (
		selected PricingRule
		found    bool
	)
	for _, rule := range rules {
		if strings.TrimSpace(rule.ModelID) != modelID {
			continue
		}
		if rule.EffectiveFromUnix > 0 && rule.EffectiveFromUnix > now.Unix() {
			continue
		}
		if !found || rule.EffectiveFromUnix > selected.EffectiveFromUnix || (rule.EffectiveFromUnix == selected.EffectiveFromUnix && strings.Compare(rule.Version, selected.Version) > 0) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func selectCurrentAgreements(now time.Time, docs []AgreementDocument) []AgreementDocument {
	best := make(map[string]AgreementDocument, len(docs))
	for _, doc := range docs {
		key := strings.TrimSpace(doc.Key)
		if key == "" {
			continue
		}
		if doc.EffectiveFromUnix > 0 && doc.EffectiveFromUnix > now.Unix() {
			continue
		}
		current, exists := best[key]
		if !exists || doc.EffectiveFromUnix > current.EffectiveFromUnix || (doc.EffectiveFromUnix == current.EffectiveFromUnix && strings.Compare(doc.Version, current.Version) > 0) {
			best[key] = doc
		}
	}
	keys := make([]string, 0, len(best))
	for key := range best {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]AgreementDocument, 0, len(keys))
	for _, key := range keys {
		items = append(items, best[key])
	}
	return items
}
