package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	AdminRoleSuperAdmin = "super_admin"
	AdminRoleReadOnly   = "read_only"
	AdminRoleFinance    = "finance"
	AdminRoleGovernance = "governance"
	AdminRoleOperations = "operations"
)

const (
	AdminCapabilityDashboardRead      = "dashboard.read"
	AdminCapabilityUsersRead          = "users.read"
	AdminCapabilityUsersWrite         = "users.write"
	AdminCapabilityOperatorsRead      = "operators.read"
	AdminCapabilityOperatorsWrite     = "operators.write"
	AdminCapabilityOrdersRead         = "orders.read"
	AdminCapabilityOrdersWrite        = "orders.write"
	AdminCapabilityWalletRead         = "wallet.read"
	AdminCapabilityWalletWrite        = "wallet.write"
	AdminCapabilityModelsRead         = "models.read"
	AdminCapabilityModelsWrite        = "models.write"
	AdminCapabilityRoutesRead         = "routes.read"
	AdminCapabilityRoutesWrite        = "routes.write"
	AdminCapabilityPricingRead        = "pricing.read"
	AdminCapabilityPricingWrite       = "pricing.write"
	AdminCapabilityAgreementsRead     = "agreements.read"
	AdminCapabilityAgreementsWrite    = "agreements.write"
	AdminCapabilityUsageRead          = "usage.read"
	AdminCapabilityAuditRead          = "audit.read"
	AdminCapabilityRefundsRead        = "refunds.read"
	AdminCapabilityRefundsReview      = "refunds.review"
	AdminCapabilityInfringementRead   = "infringement.read"
	AdminCapabilityInfringementReview = "infringement.review"
	AdminCapabilityNoticesRead        = "notices.read"
	AdminCapabilityNoticesWrite       = "notices.write"
	AdminCapabilityRiskRead           = "risk.read"
	AdminCapabilityRiskWrite          = "risk.write"
	AdminCapabilityRetentionRead      = "retention.read"
	AdminCapabilityRetentionWrite     = "retention.write"
	AdminCapabilityRuntimeRead        = "runtime.read"
	AdminCapabilityRuntimeWrite       = "runtime.write"
)

var allAdminCapabilities = []string{
	AdminCapabilityDashboardRead,
	AdminCapabilityUsersRead,
	AdminCapabilityUsersWrite,
	AdminCapabilityOperatorsRead,
	AdminCapabilityOperatorsWrite,
	AdminCapabilityOrdersRead,
	AdminCapabilityOrdersWrite,
	AdminCapabilityWalletRead,
	AdminCapabilityWalletWrite,
	AdminCapabilityModelsRead,
	AdminCapabilityModelsWrite,
	AdminCapabilityRoutesRead,
	AdminCapabilityRoutesWrite,
	AdminCapabilityPricingRead,
	AdminCapabilityPricingWrite,
	AdminCapabilityAgreementsRead,
	AdminCapabilityAgreementsWrite,
	AdminCapabilityUsageRead,
	AdminCapabilityAuditRead,
	AdminCapabilityRefundsRead,
	AdminCapabilityRefundsReview,
	AdminCapabilityInfringementRead,
	AdminCapabilityInfringementReview,
	AdminCapabilityNoticesRead,
	AdminCapabilityNoticesWrite,
	AdminCapabilityRiskRead,
	AdminCapabilityRiskWrite,
	AdminCapabilityRetentionRead,
	AdminCapabilityRetentionWrite,
	AdminCapabilityRuntimeRead,
	AdminCapabilityRuntimeWrite,
}

var roleCapabilities = map[string][]string{
	AdminRoleSuperAdmin: allAdminCapabilities,
	AdminRoleReadOnly: {
		AdminCapabilityDashboardRead,
		AdminCapabilityUsersRead,
		AdminCapabilityOperatorsRead,
		AdminCapabilityOrdersRead,
		AdminCapabilityWalletRead,
		AdminCapabilityModelsRead,
		AdminCapabilityPricingRead,
		AdminCapabilityAgreementsRead,
		AdminCapabilityUsageRead,
		AdminCapabilityAuditRead,
		AdminCapabilityRefundsRead,
		AdminCapabilityInfringementRead,
		AdminCapabilityNoticesRead,
		AdminCapabilityRiskRead,
		AdminCapabilityRetentionRead,
	},
	AdminRoleFinance: {
		AdminCapabilityDashboardRead,
		AdminCapabilityUsersRead,
		AdminCapabilityOrdersRead,
		AdminCapabilityOrdersWrite,
		AdminCapabilityWalletRead,
		AdminCapabilityWalletWrite,
		AdminCapabilityAuditRead,
		AdminCapabilityRefundsRead,
		AdminCapabilityRefundsReview,
	},
	AdminRoleGovernance: {
		AdminCapabilityDashboardRead,
		AdminCapabilityUsersRead,
		AdminCapabilityAgreementsRead,
		AdminCapabilityAgreementsWrite,
		AdminCapabilityAuditRead,
		AdminCapabilityRefundsRead,
		AdminCapabilityRefundsReview,
		AdminCapabilityInfringementRead,
		AdminCapabilityInfringementReview,
		AdminCapabilityNoticesRead,
		AdminCapabilityNoticesWrite,
		AdminCapabilityRiskRead,
		AdminCapabilityRiskWrite,
		AdminCapabilityRetentionRead,
		AdminCapabilityRetentionWrite,
	},
	AdminRoleOperations: {
		AdminCapabilityDashboardRead,
		AdminCapabilityUsersRead,
		AdminCapabilityModelsRead,
		AdminCapabilityModelsWrite,
		AdminCapabilityRoutesRead,
		AdminCapabilityRoutesWrite,
		AdminCapabilityPricingRead,
		AdminCapabilityPricingWrite,
		AdminCapabilityAgreementsRead,
		AdminCapabilityAgreementsWrite,
		AdminCapabilityUsageRead,
		AdminCapabilityAuditRead,
		AdminCapabilityRuntimeRead,
		AdminCapabilityRuntimeWrite,
	},
}

var validAdminRoles = map[string]struct{}{
	AdminRoleSuperAdmin: {},
	AdminRoleReadOnly:   {},
	AdminRoleFinance:    {},
	AdminRoleGovernance: {},
	AdminRoleOperations: {},
}

var validAdminCapabilities = func() map[string]struct{} {
	items := make(map[string]struct{}, len(allAdminCapabilities))
	for _, capability := range allAdminCapabilities {
		items[capability] = struct{}{}
	}
	return items
}()

type AdminOperator struct {
	UserID       string   `json:"user_id,omitempty"`
	Email        string   `json:"email"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities,omitempty"`
	Active       bool     `json:"active"`
	CreatedUnix  int64    `json:"created_unix,omitempty"`
	UpdatedUnix  int64    `json:"updated_unix,omitempty"`
}

func (o AdminOperator) HasCapability(capability string) bool {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return true
	}
	for _, item := range o.Capabilities {
		if strings.TrimSpace(item) == capability {
			return true
		}
	}
	return false
}

type AdminDashboardTotals struct {
	Users               int   `json:"users"`
	PaidOrders          int   `json:"paid_orders"`
	WalletBalanceFen    int64 `json:"wallet_balance_fen"`
	RefundPending       int   `json:"refund_pending"`
	InfringementPending int   `json:"infringement_pending"`
}

type AdminDashboardRecent struct {
	RechargeFen7D    int64 `json:"recharge_fen_7d"`
	ConsumptionFen7D int64 `json:"consumption_fen_7d"`
	NewUsers7D       int   `json:"new_users_7d"`
	WindowDays       int   `json:"window_days,omitempty"`
}

type AdminDashboardModelStat struct {
	ModelID          string `json:"model_id"`
	UsageCount       int    `json:"usage_count"`
	ChargedFen       int64  `json:"charged_fen"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

type AdminDashboard struct {
	Totals        AdminDashboardTotals      `json:"totals"`
	Recent        AdminDashboardRecent      `json:"recent"`
	TopModels     []AdminDashboardModelStat `json:"top_models"`
	GeneratedUnix int64                     `json:"generated_unix"`
}

type AdminUserOverview struct {
	User                     UserSummary           `json:"user"`
	Wallet                   WalletSummary         `json:"wallet"`
	RecentOrders             []RechargeOrder       `json:"recent_orders"`
	RecentTransactions       []WalletTransaction   `json:"recent_transactions"`
	Agreements               []AgreementAcceptance `json:"agreements"`
	RecentUsage              []ChatUsageRecord     `json:"recent_usage"`
	PendingRefundCount       int                   `json:"pending_refund_count"`
	PendingInfringementCount int                   `json:"pending_infringement_count"`
}

type ChatUsageRecordFilter struct {
	UserID    string
	ModelID   string
	SinceUnix int64
	Limit     int
	Offset    int
}

type AdminDashboardStoreInput struct {
	ExcludedAdminUserIDs []string
	ExcludedAdminEmails  []string
	SinceUnix            int64
}

type adminDashboardStore interface {
	BuildAdminDashboard(ctx context.Context, input AdminDashboardStoreInput) (AdminDashboard, error)
}

func defaultCapabilitiesForRole(role string) []string {
	role = strings.ToLower(strings.TrimSpace(role))
	if items, ok := roleCapabilities[role]; ok {
		return append([]string(nil), items...)
	}
	return nil
}

func DefaultAdminCapabilitiesForRole(role string) []string {
	return defaultCapabilitiesForRole(role)
}

func normalizeAdminOperator(operator AdminOperator) AdminOperator {
	operator.UserID = strings.TrimSpace(operator.UserID)
	operator.Email = strings.ToLower(strings.TrimSpace(operator.Email))
	operator.Role = strings.ToLower(strings.TrimSpace(operator.Role))
	if len(operator.Capabilities) == 0 {
		operator.Capabilities = defaultCapabilitiesForRole(operator.Role)
	}
	operator.Capabilities = uniqueSortedStrings(operator.Capabilities)
	return operator
}

func NormalizeAdminOperator(operator AdminOperator) AdminOperator {
	return normalizeAdminOperator(operator)
}

func normalizeLoadedAdminOperator(operator AdminOperator) AdminOperator {
	operator = normalizeAdminOperator(operator)
	if operator.Role == "" && !operator.Active {
		operator.Role = AdminRoleSuperAdmin
	}
	if len(operator.Capabilities) == 0 && !operator.Active && isValidAdminRole(operator.Role) {
		operator.Capabilities = defaultCapabilitiesForRole(operator.Role)
		operator.Capabilities = uniqueSortedStrings(operator.Capabilities)
	}
	return operator
}

func uniqueSortedStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func isValidAdminRole(role string) bool {
	_, ok := validAdminRoles[strings.ToLower(strings.TrimSpace(role))]
	return ok
}

func validateAdminCapabilities(items []string) error {
	for _, item := range items {
		if _, ok := validAdminCapabilities[item]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidAdminCapability, item)
		}
	}
	return nil
}

func validateAdminOperator(operator AdminOperator) error {
	if strings.TrimSpace(operator.Email) == "" {
		return fmt.Errorf("%w: operator email is required", ErrAdminAccessDenied)
	}
	if !isValidAdminRole(operator.Role) {
		return fmt.Errorf("%w: %s", ErrInvalidAdminRole, strings.TrimSpace(operator.Role))
	}
	if err := validateAdminCapabilities(operator.Capabilities); err != nil {
		return err
	}
	return nil
}

func (s *Service) SyncAdminUsers(ctx context.Context, emails []string) error {
	return s.store.UpsertAdminEmails(ctx, emails)
}

func (s *Service) GetAdminOperator(ctx context.Context, userID, email string) (AdminOperator, error) {
	operator, err := s.store.GetAdminOperator(ctx, strings.TrimSpace(userID), strings.TrimSpace(email))
	if err != nil {
		return AdminOperator{}, err
	}
	operator = normalizeLoadedAdminOperator(operator)
	if operator.Email == "" || !operator.Active {
		return AdminOperator{}, ErrAdminAccessDenied
	}
	if err := validateAdminOperator(operator); err != nil {
		return AdminOperator{}, ErrAdminAccessDenied
	}
	if userID = strings.TrimSpace(userID); userID != "" && operator.UserID != "" && operator.UserID != userID {
		return AdminOperator{}, ErrAdminAccessDenied
	}
	if userID = strings.TrimSpace(userID); userID != "" && operator.UserID == "" {
		operator.UserID = userID
		saved, err := s.store.SaveAdminOperator(ctx, operator)
		if err != nil {
			return AdminOperator{}, err
		}
		operator = normalizeLoadedAdminOperator(saved)
		if validateAdminOperator(operator) != nil {
			return AdminOperator{}, ErrAdminAccessDenied
		}
	}
	return operator, nil
}

func (s *Service) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	operator, err := s.GetAdminOperator(ctx, userID, email)
	if err != nil {
		if errors.Is(err, ErrAdminAccessDenied) {
			return false, nil
		}
		return false, err
	}
	return operator.Active, nil
}

func (s *Service) ListAdminOperators(ctx context.Context) ([]AdminOperator, error) {
	items, err := s.store.ListAdminOperators(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminOperator, 0, len(items))
	for _, item := range items {
		normalized := normalizeLoadedAdminOperator(item)
		if err := validateAdminOperator(normalized); err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active == out[j].Active {
			return out[i].Email < out[j].Email
		}
		return out[i].Active && !out[j].Active
	})
	return out, nil
}

func (s *Service) SaveAdminOperator(ctx context.Context, actor AdminActor, operator AdminOperator) (AdminOperator, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	actor.Email = strings.TrimSpace(actor.Email)
	operator = normalizeAdminOperator(operator)
	if err := validateAdminOperator(operator); err != nil {
		return AdminOperator{}, err
	}
	if operator.CreatedUnix == 0 {
		operator.CreatedUnix = s.now().Unix()
	}
	operator.UpdatedUnix = s.now().Unix()
	saved, err := s.store.SaveAdminOperator(ctx, operator)
	if err != nil {
		return AdminOperator{}, err
	}
	saved = normalizeAdminOperator(saved)
	if err := validateAdminOperator(saved); err != nil {
		return AdminOperator{}, err
	}
	_ = s.appendAuditLog(ctx, AdminAuditLog{
		ActorUserID: actor.UserID,
		ActorEmail:  actor.Email,
		Action:      "admin.operator.updated",
		TargetType:  "admin_operator",
		TargetID:    saved.Email,
		RiskLevel:   "high",
		Detail:      fmt.Sprintf("role=%s active=%t capabilities=%s", saved.Role, saved.Active, strings.Join(saved.Capabilities, ",")),
		CreatedUnix: s.now().Unix(),
	})
	return saved, nil
}

func (s *Service) RequireAdminCapability(ctx context.Context, userID, email, capability string) (AdminOperator, error) {
	operator, err := s.GetAdminOperator(ctx, userID, email)
	if err != nil {
		return AdminOperator{}, err
	}
	if !operator.HasCapability(capability) {
		return AdminOperator{}, fmt.Errorf("%w: %s", ErrAdminCapabilityDenied, strings.TrimSpace(capability))
	}
	return operator, nil
}

func (s *Service) ListChatUsageRecords(ctx context.Context, filter ChatUsageRecordFilter) ([]ChatUsageRecord, error) {
	return s.store.ListChatUsageRecords(ctx, filter)
}

func (s *Service) ListUserWalletTransactions(ctx context.Context, userID string, limit, offset int) ([]WalletTransaction, error) {
	items, err := s.store.ListTransactions(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	return applyWindow(items, offset, limit), nil
}

func (s *Service) ListUserAgreementAcceptances(ctx context.Context, userID string) ([]AgreementAcceptance, error) {
	return s.store.ListAgreementAcceptances(ctx, strings.TrimSpace(userID))
}

func (s *Service) GetAdminDashboard(ctx context.Context) (AdminDashboard, error) {
	return s.GetAdminDashboardForWindow(ctx, 7)
}

func (s *Service) GetAdminDashboardForWindow(ctx context.Context, windowDays int) (AdminDashboard, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	now := s.now()
	windowStart := now.Add(-time.Duration(windowDays) * 24 * time.Hour).Unix()
	operators, err := s.ListAdminOperators(ctx)
	if err != nil {
		return AdminDashboard{}, err
	}
	adminUserIDs := map[string]struct{}{}
	adminEmails := map[string]struct{}{}
	for _, operator := range operators {
		if !operator.Active {
			continue
		}
		if operator.UserID != "" {
			adminUserIDs[operator.UserID] = struct{}{}
		}
		if operator.Email != "" {
			adminEmails[strings.ToLower(strings.TrimSpace(operator.Email))] = struct{}{}
		}
	}
	if store, ok := s.store.(adminDashboardStore); ok {
		dashboard, err := store.BuildAdminDashboard(ctx, AdminDashboardStoreInput{
			ExcludedAdminUserIDs: sortedStringKeys(adminUserIDs),
			ExcludedAdminEmails:  sortedStringKeys(adminEmails),
			SinceUnix:            windowStart,
		})
		if err != nil {
			return AdminDashboard{}, err
		}
		dashboard.GeneratedUnix = now.Unix()
		dashboard.Recent.WindowDays = windowDays
		if dashboard.TopModels == nil {
			dashboard.TopModels = []AdminDashboardModelStat{}
		}
		return dashboard, nil
	}
	users, err := s.ListUsers(ctx, UserSummaryFilter{})
	if err != nil {
		return AdminDashboard{}, err
	}
	orders, err := s.ListOrders(ctx, RechargeOrderFilter{})
	if err != nil {
		return AdminDashboard{}, err
	}
	refunds, err := s.ListRefundRequests(ctx, RefundRequestFilter{})
	if err != nil {
		return AdminDashboard{}, err
	}
	reports, err := s.ListInfringementReports(ctx, InfringementReportFilter{})
	if err != nil {
		return AdminDashboard{}, err
	}
	usage, err := s.ListChatUsageRecords(ctx, ChatUsageRecordFilter{SinceUnix: windowStart})
	if err != nil {
		return AdminDashboard{}, err
	}

	dashboard := AdminDashboard{GeneratedUnix: now.Unix()}
	dashboard.Recent.WindowDays = windowDays
	modelStats := map[string]*AdminDashboardModelStat{}
	for _, user := range users {
		if _, ok := adminUserIDs[user.UserID]; ok {
			continue
		}
		if _, ok := adminEmails[strings.ToLower(strings.TrimSpace(user.Email))]; ok {
			continue
		}
		dashboard.Totals.Users++
		dashboard.Totals.WalletBalanceFen += user.BalanceFen
		if user.CreatedUnix >= windowStart {
			dashboard.Recent.NewUsers7D++
		}
	}
	for _, order := range orders {
		if isPaidOrder(order) {
			dashboard.Totals.PaidOrders++
			if order.CreatedUnix >= windowStart {
				dashboard.Recent.RechargeFen7D += order.AmountFen
			}
		}
	}
	for _, refund := range refunds {
		if isPendingRefund(refund.Status) {
			dashboard.Totals.RefundPending++
		}
	}
	for _, report := range reports {
		if isPendingInfringement(report.Status) {
			dashboard.Totals.InfringementPending++
		}
	}
	for _, item := range usage {
		dashboard.Recent.ConsumptionFen7D += item.ChargedFen
		stat := modelStats[item.ModelID]
		if stat == nil {
			stat = &AdminDashboardModelStat{ModelID: item.ModelID}
			modelStats[item.ModelID] = stat
		}
		stat.UsageCount++
		stat.ChargedFen += item.ChargedFen
		stat.PromptTokens += item.PromptTokens
		stat.CompletionTokens += item.CompletionTokens
	}
	for _, stat := range modelStats {
		dashboard.TopModels = append(dashboard.TopModels, *stat)
	}
	sort.Slice(dashboard.TopModels, func(i, j int) bool {
		if dashboard.TopModels[i].ChargedFen == dashboard.TopModels[j].ChargedFen {
			if dashboard.TopModels[i].UsageCount == dashboard.TopModels[j].UsageCount {
				return dashboard.TopModels[i].ModelID < dashboard.TopModels[j].ModelID
			}
			return dashboard.TopModels[i].UsageCount > dashboard.TopModels[j].UsageCount
		}
		return dashboard.TopModels[i].ChargedFen > dashboard.TopModels[j].ChargedFen
	})
	return dashboard, nil
}

func sortedStringKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (s *Service) GetAdminUserOverview(ctx context.Context, userID string) (AdminUserOverview, error) {
	userID = strings.TrimSpace(userID)
	users, err := s.ListUsers(ctx, UserSummaryFilter{UserID: userID, Limit: 1})
	if err != nil {
		return AdminUserOverview{}, err
	}
	if len(users) == 0 {
		return AdminUserOverview{}, fmt.Errorf("%w: user %s", ErrUserNotFound, userID)
	}
	wallet, err := s.GetWallet(ctx, userID)
	if err != nil {
		return AdminUserOverview{}, err
	}
	orders, err := s.ListOrders(ctx, RechargeOrderFilter{UserID: userID, Limit: 5})
	if err != nil {
		return AdminUserOverview{}, err
	}
	transactions, err := s.ListUserWalletTransactions(ctx, userID, 5, 0)
	if err != nil {
		return AdminUserOverview{}, err
	}
	agreements, err := s.ListUserAgreementAcceptances(ctx, userID)
	if err != nil {
		return AdminUserOverview{}, err
	}
	usage, err := s.ListChatUsageRecords(ctx, ChatUsageRecordFilter{UserID: userID, Limit: 5})
	if err != nil {
		return AdminUserOverview{}, err
	}
	refunds, err := s.ListRefundRequests(ctx, RefundRequestFilter{UserID: userID})
	if err != nil {
		return AdminUserOverview{}, err
	}
	reports, err := s.ListInfringementReports(ctx, InfringementReportFilter{UserID: userID})
	if err != nil {
		return AdminUserOverview{}, err
	}
	return AdminUserOverview{
		User:                     users[0],
		Wallet:                   wallet,
		RecentOrders:             orders,
		RecentTransactions:       transactions,
		Agreements:               agreements,
		RecentUsage:              usage,
		PendingRefundCount:       countPendingRefunds(refunds),
		PendingInfringementCount: countPendingInfringementReports(reports),
	}, nil
}

func (s *Service) RedactAdminUserOverview(overview AdminUserOverview, operator AdminOperator) AdminUserOverview {
	if !operator.HasCapability(AdminCapabilityWalletRead) {
		overview.Wallet = WalletSummary{}
		overview.RecentTransactions = nil
	}
	if !operator.HasCapability(AdminCapabilityOrdersRead) {
		overview.RecentOrders = nil
	}
	if !operator.HasCapability(AdminCapabilityAgreementsRead) {
		overview.Agreements = nil
	}
	if !operator.HasCapability(AdminCapabilityUsageRead) {
		overview.RecentUsage = nil
	}
	if !operator.HasCapability(AdminCapabilityRefundsRead) {
		overview.PendingRefundCount = 0
	}
	if !operator.HasCapability(AdminCapabilityInfringementRead) {
		overview.PendingInfringementCount = 0
	}
	return overview
}

func filterChatUsageRecords(items []ChatUsageRecord, filter ChatUsageRecordFilter) []ChatUsageRecord {
	userID := strings.TrimSpace(filter.UserID)
	modelID := strings.TrimSpace(filter.ModelID)
	filtered := make([]ChatUsageRecord, 0, len(items))
	for _, item := range items {
		if userID != "" && item.UserID != userID {
			continue
		}
		if modelID != "" && item.ModelID != modelID {
			continue
		}
		if filter.SinceUnix > 0 && item.CreatedUnix < filter.SinceUnix {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedUnix == filtered[j].CreatedUnix {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedUnix > filtered[j].CreatedUnix
	})
	return applyWindow(filtered, filter.Offset, filter.Limit)
}

func countPendingRefunds(items []RefundRequest) int {
	total := 0
	for _, item := range items {
		if isPendingRefund(item.Status) {
			total++
		}
	}
	return total
}

func countPendingInfringementReports(items []InfringementReport) int {
	total := 0
	for _, item := range items {
		if isPendingInfringement(item.Status) {
			total++
		}
	}
	return total
}

func isPendingRefund(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "approved_pending_payout", "refund_failed":
		return true
	default:
		return false
	}
}

func isPendingInfringement(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "pending", "reviewing":
		return true
	default:
		return false
	}
}

func isPaidOrder(order RechargeOrder) bool {
	switch strings.ToLower(strings.TrimSpace(order.Status)) {
	case "paid", "refunded":
		return true
	default:
		return order.PaidUnix > 0
	}
}
