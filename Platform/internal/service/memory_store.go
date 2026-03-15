package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"openclaw/platform/internal/revisiontoken"
)

type MemoryStore struct {
	mu                sync.Mutex
	wallets           map[string]WalletSummary
	users             map[string]UserIdentity
	nextUserNo        int64
	transactions      map[string][]WalletTransaction
	transactionRefs   map[string]WalletTransaction
	orders            map[string]RechargeOrder
	adminOperators    map[string]AdminOperator
	agreements        map[string]AgreementAcceptance
	chatUsage         []ChatUsageRecord
	auditLogs         []AdminAuditLog
	refundRequests    map[string]RefundRequest
	infringements     map[string]InfringementReport
	retentionPolicies []DataRetentionPolicy
	systemNotices     []SystemNotice
	riskRules         []RiskRule
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		wallets:         map[string]WalletSummary{},
		users:           map[string]UserIdentity{},
		transactions:    map[string][]WalletTransaction{},
		transactionRefs: map[string]WalletTransaction{},
		orders:          map[string]RechargeOrder{},
		adminOperators:  map[string]AdminOperator{},
		agreements:      map[string]AgreementAcceptance{},
		refundRequests:  map[string]RefundRequest{},
		infringements:   map[string]InfringementReport{},
	}
}

func (s *MemoryStore) SetBalance(userID string, balanceFen int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureUserIdentityLocked(userID)
	s.wallets[userID] = WalletSummary{
		UserID:      userID,
		BalanceFen:  balanceFen,
		Currency:    "CNY",
		UpdatedUnix: time.Now().Unix(),
	}
}

func (s *MemoryStore) GetWallet(ctx context.Context, userID string) (WalletSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wallet, ok := s.wallets[userID]
	if !ok {
		wallet = WalletSummary{UserID: userID, Currency: "CNY", UpdatedUnix: time.Now().Unix()}
		s.wallets[userID] = wallet
	}
	return wallet, nil
}

func (s *MemoryStore) AppendTransaction(ctx context.Context, tx WalletTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendTransactionLocked(tx)
	return nil
}

func (s *MemoryStore) ListTransactions(ctx context.Context, userID string) ([]WalletTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.transactions[userID]
	out := make([]WalletTransaction, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedUnix == out[j].CreatedUnix {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedUnix > out[j].CreatedUnix
	})
	return out, nil
}

func (s *MemoryStore) SaveOrder(ctx context.Context, order RechargeOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.orders[order.ID]; ok && order.CreatedUnix == 0 {
		order.CreatedUnix = existing.CreatedUnix
	}
	if order.CreatedUnix == 0 {
		order.CreatedUnix = time.Now().Unix()
	}
	if order.UpdatedUnix == 0 {
		order.UpdatedUnix = time.Now().Unix()
	}
	s.orders[order.ID] = order
	return nil
}

func (s *MemoryStore) ListOrders(ctx context.Context) ([]RechargeOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]RechargeOrder, 0, len(s.orders))
	for _, order := range s.orders {
		identity := s.ensureUserIdentityLocked(order.UserID)
		order.UserNo = identity.UserNo
		order.Username = identity.Username
		items = append(items, order)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedUnix == items[j].CreatedUnix {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedUnix > items[j].CreatedUnix
	})
	return items, nil
}

func (s *MemoryStore) GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.UserID != userID {
		return RechargeOrder{}, fmt.Errorf("order %s not found", orderID)
	}
	identity := s.ensureUserIdentityLocked(order.UserID)
	order.UserNo = identity.UserNo
	order.Username = identity.Username
	return order, nil
}

func (s *MemoryStore) FindOrderByID(ctx context.Context, orderID string) (RechargeOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return RechargeOrder{}, fmt.Errorf("order %s not found", orderID)
	}
	identity := s.ensureUserIdentityLocked(order.UserID)
	order.UserNo = identity.UserNo
	order.Username = identity.Username
	return order, nil
}

func (s *MemoryStore) MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (RechargeOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return RechargeOrder{}, false, fmt.Errorf("order %s not found", orderID)
	}
	if order.Status == "paid" {
		return order, false, nil
	}
	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	order.PaidUnix = time.Now().Unix()
	order.UpdatedUnix = time.Now().Unix()
	s.orders[orderID] = order
	return order, true, nil
}

func (s *MemoryStore) FinalizeRechargeOrder(
	ctx context.Context,
	orderID, provider, externalID, description string,
) (RechargeOrder, WalletSummary, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.orders[orderID]
	if !ok {
		return RechargeOrder{}, WalletSummary{}, false, fmt.Errorf("order %s not found", orderID)
	}
	wallet, ok := s.wallets[order.UserID]
	if !ok {
		wallet = WalletSummary{UserID: order.UserID, Currency: "CNY"}
	}
	if order.Status == "paid" {
		return order, wallet, false, nil
	}

	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	order.PaidUnix = time.Now().Unix()
	order.UpdatedUnix = time.Now().Unix()
	s.orders[orderID] = order

	wallet.BalanceFen += order.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[order.UserID] = wallet
	s.appendTransactionLocked(WalletTransaction{
		ID:            fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:        order.UserID,
		Kind:          "credit",
		AmountFen:     order.AmountFen,
		Description:   description,
		ReferenceType: "recharge_order",
		ReferenceID:   order.ID,
		CreatedUnix:   time.Now().Unix(),
	})
	return order, wallet, true, nil
}

func (s *MemoryStore) Credit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wallet, ok := s.wallets[userID]
	if !ok {
		wallet = WalletSummary{UserID: userID, Currency: "CNY"}
	}
	wallet.BalanceFen += amountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[userID] = wallet
	s.appendTransactionLocked(WalletTransaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:      userID,
		Kind:        "credit",
		AmountFen:   amountFen,
		Description: description,
		CreatedUnix: time.Now().Unix(),
	})
	return wallet, nil
}

func (s *MemoryStore) Debit(ctx context.Context, userID string, amountFen int64, description string) (WalletSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wallet, ok := s.wallets[userID]
	if !ok {
		wallet = WalletSummary{UserID: userID, Currency: "CNY"}
	}
	if wallet.BalanceFen < amountFen {
		return WalletSummary{}, ErrInsufficientFunds
	}
	wallet.BalanceFen -= amountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[userID] = wallet
	s.appendTransactionLocked(WalletTransaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:      userID,
		Kind:        "debit",
		AmountFen:   -amountFen,
		Description: description,
		CreatedUnix: time.Now().Unix(),
	})
	return wallet, nil
}

func (s *MemoryStore) UpsertAdminEmails(ctx context.Context, emails []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		operator, ok := s.adminOperators[email]
		if !ok {
			operator = AdminOperator{
				Email:        email,
				Role:         AdminRoleSuperAdmin,
				Capabilities: defaultCapabilitiesForRole(AdminRoleSuperAdmin),
				CreatedUnix:  now,
			}
		}
		operator.Email = email
		operator.Active = true
		operator.SeedManaged = true
		if operator.Role == "" {
			operator.Role = AdminRoleSuperAdmin
		}
		if len(operator.Capabilities) == 0 {
			operator.Capabilities = defaultCapabilitiesForRole(operator.Role)
		}
		operator.UpdatedUnix = now
		s.adminOperators[email] = normalizeAdminOperator(operator)
	}
	for email, operator := range s.adminOperators {
		if !operator.SeedManaged {
			continue
		}
		if _, ok := s.adminOperators[email]; !ok {
			continue
		}
		found := false
		for _, item := range emails {
			if strings.EqualFold(strings.TrimSpace(item), email) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		operator.Active = false
		operator.UpdatedUnix = now
		s.adminOperators[email] = normalizeAdminOperator(operator)
	}
	return nil
}

func (s *MemoryStore) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operator := s.lookupAdminOperatorLocked(userID, email)
	return operator.Email != "" && operator.Active, nil
}

func (s *MemoryStore) GetAdminOperator(ctx context.Context, userID, email string) (AdminOperator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operator := s.lookupAdminOperatorLocked(userID, email)
	if operator.Email == "" {
		return AdminOperator{}, nil
	}
	return normalizeAdminOperator(operator), nil
}

func (s *MemoryStore) ListAdminOperators(ctx context.Context) ([]AdminOperator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]AdminOperator, 0, len(s.adminOperators))
	for _, operator := range s.adminOperators {
		items = append(items, normalizeAdminOperator(operator))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Active == items[j].Active {
			return items[i].Email < items[j].Email
		}
		return items[i].Active && !items[j].Active
	})
	return items, nil
}

func (s *MemoryStore) SaveAdminOperator(ctx context.Context, operator AdminOperator) (AdminOperator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operator = normalizeAdminOperator(operator)
	existing := s.adminOperators[operator.Email]
	if existing.Email != "" {
		if operator.UserID == "" {
			operator.UserID = existing.UserID
		}
		if operator.CreatedUnix == 0 {
			operator.CreatedUnix = existing.CreatedUnix
		}
		operator.SeedManaged = existing.SeedManaged
	}
	if operator.CreatedUnix == 0 {
		operator.CreatedUnix = time.Now().Unix()
	}
	if operator.UpdatedUnix == 0 {
		operator.UpdatedUnix = time.Now().Unix()
	}
	s.adminOperators[operator.Email] = operator
	return operator, nil
}

func (s *MemoryStore) RecordAgreementAcceptance(ctx context.Context, acceptance AgreementAcceptance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agreements[acceptance.UserID+":"+acceptance.AgreementKey+":"+acceptance.Version] = acceptance
	return nil
}

func (s *MemoryStore) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.agreements[userID+":"+key+":"+version]
	return ok, nil
}

func (s *MemoryStore) ListAgreementAcceptances(ctx context.Context, userID string) ([]AgreementAcceptance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]AgreementAcceptance, 0)
	for _, item := range s.agreements {
		if userID != "" && item.UserID != userID {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].AcceptedUnix == items[j].AcceptedUnix {
			if items[i].UserID == items[j].UserID {
				if items[i].AgreementKey == items[j].AgreementKey {
					return items[i].Version > items[j].Version
				}
				return items[i].AgreementKey > items[j].AgreementKey
			}
			return items[i].UserID > items[j].UserID
		}
		return items[i].AcceptedUnix > items[j].AcceptedUnix
	})
	return items, nil
}

func (s *MemoryStore) RecordChatUsage(ctx context.Context, usage ChatUsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatUsage = append(s.chatUsage, usage)
	return nil
}

func (s *MemoryStore) ListChatUsageRecords(ctx context.Context, filter ChatUsageRecordFilter) ([]ChatUsageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ChatUsageRecord, len(s.chatUsage))
	copy(items, s.chatUsage)
	return filterChatUsageRecords(items, filter), nil
}

func (s *MemoryStore) UpsertUserIdentity(ctx context.Context, identity UserIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.users[identity.UserID]
	if ok {
		if identity.UserNo == 0 {
			identity.UserNo = existing.UserNo
		}
		if strings.TrimSpace(identity.Username) == "" {
			identity.Username = existing.Username
		}
		if identity.Email == "" {
			identity.Email = existing.Email
		}
		if identity.CreatedUnix == 0 || (existing.CreatedUnix > 0 && existing.CreatedUnix < identity.CreatedUnix) {
			identity.CreatedUnix = existing.CreatedUnix
		}
		if identity.UpdatedUnix < existing.UpdatedUnix {
			identity.UpdatedUnix = existing.UpdatedUnix
		}
		if identity.LastSeenUnix < existing.LastSeenUnix {
			identity.LastSeenUnix = existing.LastSeenUnix
		}
	} else if identity.UserNo == 0 {
		identity.UserNo = s.nextUserNumberLocked()
	}
	if identity.UserNo > s.nextUserNo {
		s.nextUserNo = identity.UserNo
	}
	s.users[identity.UserID] = identity
	return nil
}

func (s *MemoryStore) ApplyWalletAdjustment(ctx context.Context, tx WalletTransaction) (WalletSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx.UserID == "" || tx.AmountFen == 0 {
		return WalletSummary{}, ErrInvalidAmount
	}
	wallet, ok := s.wallets[tx.UserID]
	if !ok {
		wallet = WalletSummary{UserID: tx.UserID, Currency: "CNY"}
	}
	if wallet.Currency == "" {
		wallet.Currency = "CNY"
	}
	if tx.AmountFen < 0 && wallet.BalanceFen < -tx.AmountFen {
		return WalletSummary{}, ErrInsufficientFunds
	}
	wallet.BalanceFen += tx.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[tx.UserID] = wallet
	if tx.ID == "" {
		tx.ID = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	if tx.Kind == "" {
		tx.Kind = walletAdjustmentKind(tx.AmountFen)
	}
	if tx.CreatedUnix == 0 {
		tx.CreatedUnix = time.Now().Unix()
	}
	s.appendTransactionLocked(tx)
	return wallet, nil
}

func (s *MemoryStore) ApplyAdminWalletMutation(
	ctx context.Context,
	tx WalletTransaction,
	audit AdminAuditLog,
) (WalletSummary, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx.UserID == "" || tx.AmountFen == 0 {
		return WalletSummary{}, false, ErrInvalidAmount
	}
	refKey := walletMutationReferenceKey(tx.ReferenceType, tx.ReferenceID)
	if refKey == "" {
		return WalletSummary{}, false, ErrInvalidRequestID
	}
	if existing, ok := s.transactionRefs[refKey]; ok {
		if !walletMutationMatches(existing, tx) {
			return WalletSummary{}, false, ErrIdempotencyConflict
		}
		wallet, ok := s.wallets[existing.UserID]
		if !ok {
			wallet = WalletSummary{UserID: existing.UserID, Currency: "CNY"}
		}
		if wallet.Currency == "" {
			wallet.Currency = "CNY"
		}
		return wallet, true, nil
	}
	wallet, ok := s.wallets[tx.UserID]
	if !ok {
		wallet = WalletSummary{UserID: tx.UserID, Currency: "CNY"}
	}
	if wallet.Currency == "" {
		wallet.Currency = "CNY"
	}
	if tx.AmountFen < 0 && wallet.BalanceFen < -tx.AmountFen {
		return WalletSummary{}, false, ErrInsufficientFunds
	}
	wallet.BalanceFen += tx.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[tx.UserID] = wallet
	if tx.ID == "" {
		tx.ID = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	if tx.Kind == "" {
		tx.Kind = walletAdjustmentKind(tx.AmountFen)
	}
	if tx.CreatedUnix == 0 {
		tx.CreatedUnix = time.Now().Unix()
	}
	s.appendTransactionLocked(tx)
	if strings.TrimSpace(audit.ID) == "" {
		audit.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}
	if audit.CreatedUnix == 0 {
		audit.CreatedUnix = time.Now().Unix()
	}
	s.appendAuditLogLocked(audit)
	return wallet, false, nil
}

func (s *MemoryStore) ListUsers(ctx context.Context) ([]UserSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for userID := range s.wallets {
		s.ensureUserIdentityLocked(userID)
	}
	summaries := make(map[string]UserSummary, len(s.users)+len(s.wallets))
	for userID, identity := range s.users {
		summaries[userID] = UserSummary{
			UserID:       userID,
			UserNo:       identity.UserNo,
			Username:     identity.Username,
			Email:        identity.Email,
			CreatedUnix:  identity.CreatedUnix,
			LastSeenUnix: identity.LastSeenUnix,
			UpdatedUnix:  maxInt64(identity.UpdatedUnix, identity.LastSeenUnix),
		}
	}
	for userID, wallet := range s.wallets {
		summary := summaries[userID]
		summary.UserID = userID
		summary.BalanceFen = wallet.BalanceFen
		summary.Currency = wallet.Currency
		summary.UpdatedUnix = maxInt64(summary.UpdatedUnix, wallet.UpdatedUnix)
		summaries[userID] = summary
	}
	for userID, summary := range summaries {
		for _, order := range s.orders {
			if order.UserID != userID {
				continue
			}
			summary.OrderCount++
			if order.CreatedUnix > summary.LastOrderUnix {
				summary.LastOrderUnix = order.CreatedUnix
			}
		}
		for _, refund := range s.refundRequests {
			if refund.UserID != userID {
				continue
			}
			summary.RefundCount++
			if refund.CreatedUnix > summary.LastRefundUnix {
				summary.LastRefundUnix = refund.CreatedUnix
			}
		}
		if summary.Currency == "" {
			summary.Currency = "CNY"
		}
		summaries[userID] = summary
	}
	items := make([]UserSummary, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedUnix == items[j].UpdatedUnix {
			if items[i].UserNo > 0 && items[j].UserNo > 0 && items[i].UserNo != items[j].UserNo {
				return items[i].UserNo < items[j].UserNo
			}
			return items[i].UserID < items[j].UserID
		}
		return items[i].UpdatedUnix > items[j].UpdatedUnix
	})
	return items, nil
}

func (s *MemoryStore) ensureUserIdentityLocked(userID string) UserIdentity {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return UserIdentity{}
	}
	identity, ok := s.users[userID]
	if ok {
		if identity.UserNo == 0 {
			identity.UserNo = s.nextUserNumberLocked()
			s.users[userID] = identity
		}
		return identity
	}
	identity = UserIdentity{
		UserID: userID,
		UserNo: s.nextUserNumberLocked(),
	}
	s.users[userID] = identity
	return identity
}

func (s *MemoryStore) nextUserNumberLocked() int64 {
	s.nextUserNo++
	return s.nextUserNo
}

func (s *MemoryStore) ListWalletAdjustments(ctx context.Context) ([]WalletTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]WalletTransaction, 0)
	for _, txs := range s.transactions {
		for _, tx := range txs {
			identity := s.ensureUserIdentityLocked(tx.UserID)
			tx.UserNo = identity.UserNo
			tx.Username = identity.Username
			items = append(items, tx)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedUnix == items[j].CreatedUnix {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedUnix > items[j].CreatedUnix
	})
	return items, nil
}

func (s *MemoryStore) AppendAuditLog(ctx context.Context, entry AdminAuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendAuditLogLocked(entry)
	return nil
}

func (s *MemoryStore) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]AdminAuditLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]AdminAuditLog, 0, len(s.auditLogs))
	for _, item := range s.auditLogs {
		if filter.Action != "" && item.Action != filter.Action {
			continue
		}
		if filter.TargetType != "" && item.TargetType != filter.TargetType {
			continue
		}
		if filter.TargetID != "" && item.TargetID != filter.TargetID {
			continue
		}
		if filter.ActorUserID != "" && item.ActorUserID != filter.ActorUserID {
			continue
		}
		if filter.RiskLevel != "" && strings.ToLower(strings.TrimSpace(item.RiskLevel)) != strings.ToLower(strings.TrimSpace(filter.RiskLevel)) {
			continue
		}
		if filter.SinceUnix > 0 && item.CreatedUnix < filter.SinceUnix {
			continue
		}
		if filter.UntilUnix > 0 && item.CreatedUnix > filter.UntilUnix {
			continue
		}
		items = append(items, item)
	}
	return applyWindow(items, filter.Offset, filter.Limit), nil
}

func (s *MemoryStore) CreateRefundRequest(ctx context.Context, request RefundRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refundRequests[request.ID] = request
	return nil
}

func (s *MemoryStore) SaveRefundRequest(ctx context.Context, request RefundRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refundRequests[request.ID] = request
	return nil
}

func (s *MemoryStore) GetRefundRequest(ctx context.Context, requestID string) (RefundRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.refundRequests[requestID]
	if !ok {
		return RefundRequest{}, fmt.Errorf("refund request %s not found", requestID)
	}
	identity := s.ensureUserIdentityLocked(item.UserID)
	item.UserNo = identity.UserNo
	item.Username = identity.Username
	return item, nil
}

func (s *MemoryStore) ListRefundRequests(ctx context.Context, userID string) ([]RefundRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]RefundRequest, 0, len(s.refundRequests))
	for _, item := range s.refundRequests {
		if userID != "" && item.UserID != userID {
			continue
		}
		identity := s.ensureUserIdentityLocked(item.UserID)
		item.UserNo = identity.UserNo
		item.Username = identity.Username
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedUnix == items[j].CreatedUnix {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedUnix > items[j].CreatedUnix
	})
	return items, nil
}

func (s *MemoryStore) ApplyRefundDecision(ctx context.Context, requestID string, input RefundDecisionInput, updatedUnix int64) (RefundRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.refundRequests[requestID]
	if !ok {
		return RefundRequest{}, fmt.Errorf("refund request %s not found", requestID)
	}
	if request.Status != "pending" && request.Status != "approved_pending_payout" {
		return RefundRequest{}, fmt.Errorf("%w: refund request %s is already %s", ErrRefundNotAllowed, requestID, request.Status)
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case "refunded", "rejected", "refund_failed", "approved_pending_payout":
	default:
		return RefundRequest{}, fmt.Errorf("%w: unsupported refund decision status %s", ErrRefundNotAllowed, status)
	}
	request.Status = status
	request.ReviewNote = strings.TrimSpace(input.ReviewNote)
	request.ReviewedBy = strings.TrimSpace(input.ReviewedBy)
	request.RefundProvider = strings.TrimSpace(input.RefundProvider)
	request.ExternalRefundID = strings.TrimSpace(input.ExternalRefundID)
	request.ExternalStatus = strings.TrimSpace(input.ExternalStatus)
	request.FailureReason = strings.TrimSpace(input.FailureReason)
	request.UpdatedUnix = updatedUnix
	allowNegativeBalance := strings.HasPrefix(strings.ToLower(request.ReviewedBy), "system:")
	if status == "refunded" && request.SettledUnix == 0 {
		order, ok := s.orders[request.OrderID]
		if !ok {
			return RefundRequest{}, fmt.Errorf("order %s not found", request.OrderID)
		}
		remainingRefundable := order.AmountFen - order.RefundedFen
		if remainingRefundable <= 0 || request.AmountFen > remainingRefundable {
			return RefundRequest{}, fmt.Errorf("%w: order %s only has %d fen refundable", ErrRefundNotAllowed, request.OrderID, remainingRefundable)
		}
		wallet, ok := s.wallets[request.UserID]
		if !ok {
			wallet = WalletSummary{UserID: request.UserID, Currency: "CNY"}
		}
		if !allowNegativeBalance && wallet.BalanceFen < request.AmountFen {
			return RefundRequest{}, fmt.Errorf("%w: wallet balance %d fen is lower than requested refund %d fen", ErrRefundNotAllowed, wallet.BalanceFen, request.AmountFen)
		}
		wallet.BalanceFen -= request.AmountFen
		wallet.UpdatedUnix = updatedUnix
		s.wallets[request.UserID] = wallet
		order.RefundedFen += request.AmountFen
		if order.RefundedFen > order.AmountFen {
			order.RefundedFen = order.AmountFen
		}
		if order.RefundedFen == order.AmountFen {
			order.Status = "refunded"
		}
		order.UpdatedUnix = updatedUnix
		s.orders[request.OrderID] = order
		s.appendTransactionLocked(WalletTransaction{
			ID:            fmt.Sprintf("tx_%d", time.Now().UnixNano()),
			UserID:        request.UserID,
			Kind:          "refund",
			AmountFen:     -request.AmountFen,
			Description:   "refund payout",
			ReferenceType: "refund_request",
			ReferenceID:   request.ID,
			CreatedUnix:   updatedUnix,
		})
		request.SettledUnix = updatedUnix
	}
	s.refundRequests[requestID] = request
	return request, nil
}

func (s *MemoryStore) CreateInfringementReport(ctx context.Context, report InfringementReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.infringements[report.ID] = report
	return nil
}

func (s *MemoryStore) SaveInfringementReport(ctx context.Context, report InfringementReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.infringements[report.ID] = report
	return nil
}

func (s *MemoryStore) GetInfringementReport(ctx context.Context, reportID string) (InfringementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.infringements[reportID]
	if !ok {
		return InfringementReport{}, fmt.Errorf("infringement report %s not found", reportID)
	}
	identity := s.ensureUserIdentityLocked(item.UserID)
	item.UserNo = identity.UserNo
	item.Username = identity.Username
	return item, nil
}

func (s *MemoryStore) ListInfringementReports(ctx context.Context, userID string) ([]InfringementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]InfringementReport, 0, len(s.infringements))
	for _, item := range s.infringements {
		if userID != "" && item.UserID != userID {
			continue
		}
		identity := s.ensureUserIdentityLocked(item.UserID)
		item.UserNo = identity.UserNo
		item.Username = identity.Username
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedUnix == items[j].CreatedUnix {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedUnix > items[j].CreatedUnix
	})
	return items, nil
}

func (s *MemoryStore) ListDataRetentionPolicies(ctx context.Context) ([]DataRetentionPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DataRetentionPolicy(nil), s.retentionPolicies...), nil
}

func (s *MemoryStore) SaveDataRetentionPolicies(ctx context.Context, policies []DataRetentionPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retentionPolicies = append([]DataRetentionPolicy(nil), policies...)
	return nil
}

func (s *MemoryStore) SaveDataRetentionPoliciesWithRevision(ctx context.Context, expectedRevision string, policies []DataRetentionPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateMemoryRevision(expectedRevision, s.retentionPolicies); err != nil {
		return err
	}
	s.retentionPolicies = append([]DataRetentionPolicy(nil), policies...)
	return nil
}

func (s *MemoryStore) ListSystemNotices(ctx context.Context) ([]SystemNotice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SystemNotice(nil), s.systemNotices...), nil
}

func (s *MemoryStore) SaveSystemNotices(ctx context.Context, notices []SystemNotice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.systemNotices = append([]SystemNotice(nil), notices...)
	return nil
}

func (s *MemoryStore) SaveSystemNoticesWithRevision(ctx context.Context, expectedRevision string, notices []SystemNotice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateMemoryRevision(expectedRevision, s.systemNotices); err != nil {
		return err
	}
	s.systemNotices = append([]SystemNotice(nil), notices...)
	return nil
}

func (s *MemoryStore) ListRiskRules(ctx context.Context) ([]RiskRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RiskRule(nil), s.riskRules...), nil
}

func (s *MemoryStore) SaveRiskRules(ctx context.Context, rules []RiskRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.riskRules = append([]RiskRule(nil), rules...)
	return nil
}

func (s *MemoryStore) SaveRiskRulesWithRevision(ctx context.Context, expectedRevision string, rules []RiskRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateMemoryRevision(expectedRevision, s.riskRules); err != nil {
		return err
	}
	s.riskRules = append([]RiskRule(nil), rules...)
	return nil
}

func validateMemoryRevision[T any](expected string, payload []T) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	current, err := revisiontoken.ForPayload(payload)
	if err != nil {
		return err
	}
	if revisiontoken.Matches(expected, current) {
		return nil
	}
	return ErrRevisionConflict
}

func (s *MemoryStore) lookupAdminOperatorLocked(userID, email string) AdminOperator {
	email = strings.ToLower(strings.TrimSpace(email))
	userID = strings.TrimSpace(userID)
	if userID != "" {
		for _, operator := range s.adminOperators {
			if operator.UserID == userID {
				return operator
			}
		}
	}
	if email != "" {
		if operator, ok := s.adminOperators[email]; ok {
			if operator.UserID != "" && userID != "" && operator.UserID != userID {
				return AdminOperator{}
			}
			return operator
		}
	}
	return AdminOperator{}
}

func walletMutationReferenceKey(referenceType, referenceID string) string {
	referenceType = strings.TrimSpace(referenceType)
	referenceID = strings.TrimSpace(referenceID)
	if referenceType == "" || referenceID == "" {
		return ""
	}
	return referenceType + "::" + referenceID
}

func walletMutationMatches(existing, current WalletTransaction) bool {
	return strings.TrimSpace(existing.UserID) == strings.TrimSpace(current.UserID) &&
		strings.TrimSpace(existing.Kind) == strings.TrimSpace(current.Kind) &&
		existing.AmountFen == current.AmountFen &&
		strings.TrimSpace(existing.Description) == strings.TrimSpace(current.Description) &&
		strings.TrimSpace(existing.ReferenceType) == strings.TrimSpace(current.ReferenceType) &&
		strings.TrimSpace(existing.ReferenceID) == strings.TrimSpace(current.ReferenceID)
}

func (s *MemoryStore) appendTransactionLocked(tx WalletTransaction) {
	s.transactions[tx.UserID] = append(s.transactions[tx.UserID], tx)
	if refKey := walletMutationReferenceKey(tx.ReferenceType, tx.ReferenceID); refKey != "" {
		s.transactionRefs[refKey] = tx
	}
}

func (s *MemoryStore) appendAuditLogLocked(entry AdminAuditLog) {
	s.auditLogs = append(s.auditLogs, entry)
	sort.Slice(s.auditLogs, func(i, j int) bool {
		if s.auditLogs[i].CreatedUnix == s.auditLogs[j].CreatedUnix {
			return s.auditLogs[i].ID > s.auditLogs[j].ID
		}
		return s.auditLogs[i].CreatedUnix > s.auditLogs[j].CreatedUnix
	})
}
