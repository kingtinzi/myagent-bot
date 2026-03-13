package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu                sync.Mutex
	wallets           map[string]WalletSummary
	transactions      map[string][]WalletTransaction
	orders            map[string]RechargeOrder
	adminEmails       map[string]struct{}
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
		wallets:        map[string]WalletSummary{},
		transactions:   map[string][]WalletTransaction{},
		orders:         map[string]RechargeOrder{},
		adminEmails:    map[string]struct{}{},
		agreements:     map[string]AgreementAcceptance{},
		refundRequests: map[string]RefundRequest{},
		infringements:  map[string]InfringementReport{},
	}
}

func (s *MemoryStore) SetBalance(userID string, balanceFen int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.transactions[tx.UserID] = append(s.transactions[tx.UserID], tx)
	return nil
}

func (s *MemoryStore) ListTransactions(ctx context.Context, userID string) ([]WalletTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.transactions[userID]
	out := make([]WalletTransaction, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedUnix > out[j].CreatedUnix })
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
		items = append(items, order)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedUnix > items[j].CreatedUnix })
	return items, nil
}

func (s *MemoryStore) GetOrder(ctx context.Context, userID, orderID string) (RechargeOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.UserID != userID {
		return RechargeOrder{}, fmt.Errorf("order %s not found", orderID)
	}
	return order, nil
}

func (s *MemoryStore) FindOrderByID(ctx context.Context, orderID string) (RechargeOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return RechargeOrder{}, fmt.Errorf("order %s not found", orderID)
	}
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
	s.transactions[order.UserID] = append(s.transactions[order.UserID], WalletTransaction{
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
	s.transactions[userID] = append(s.transactions[userID], WalletTransaction{
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
	s.transactions[userID] = append(s.transactions[userID], WalletTransaction{
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
	s.adminEmails = map[string]struct{}{}
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		s.adminEmails[email] = struct{}{}
	}
	return nil
}

func (s *MemoryStore) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.adminEmails[strings.ToLower(strings.TrimSpace(email))]
	return ok, nil
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
	sort.Slice(items, func(i, j int) bool { return items[i].AcceptedUnix > items[j].AcceptedUnix })
	return items, nil
}

func (s *MemoryStore) RecordChatUsage(ctx context.Context, usage ChatUsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatUsage = append(s.chatUsage, usage)
	return nil
}

func (s *MemoryStore) ListUsers(ctx context.Context) ([]UserSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]UserSummary, 0, len(s.wallets))
	for userID, wallet := range s.wallets {
		summary := UserSummary{
			UserID:      userID,
			BalanceFen:  wallet.BalanceFen,
			Currency:    wallet.Currency,
			UpdatedUnix: wallet.UpdatedUnix,
		}
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
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedUnix > items[j].UpdatedUnix })
	return items, nil
}

func (s *MemoryStore) ListWalletAdjustments(ctx context.Context) ([]WalletTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]WalletTransaction, 0)
	for _, txs := range s.transactions {
		items = append(items, txs...)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedUnix > items[j].CreatedUnix })
	return items, nil
}

func (s *MemoryStore) AppendAuditLog(ctx context.Context, entry AdminAuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs = append(s.auditLogs, entry)
	sort.Slice(s.auditLogs, func(i, j int) bool { return s.auditLogs[i].CreatedUnix > s.auditLogs[j].CreatedUnix })
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
		if filter.ActorUserID != "" && item.ActorUserID != filter.ActorUserID {
			continue
		}
		items = append(items, item)
	}
	return items, nil
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
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedUnix > items[j].CreatedUnix })
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
	if status == "approved" {
		status = "refunded"
	}
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
	if status == "refunded" && request.SettledUnix == 0 {
		order, ok := s.orders[request.OrderID]
		if !ok {
			return RefundRequest{}, fmt.Errorf("order %s not found", request.OrderID)
		}
		wallet, ok := s.wallets[request.UserID]
		if !ok {
			wallet = WalletSummary{UserID: request.UserID, Currency: "CNY"}
		}
		if wallet.BalanceFen < request.AmountFen {
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
		s.transactions[request.UserID] = append(s.transactions[request.UserID], WalletTransaction{
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
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedUnix > items[j].CreatedUnix })
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
