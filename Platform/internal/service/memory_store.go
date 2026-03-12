package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu           sync.Mutex
	wallets      map[string]WalletSummary
	transactions map[string][]WalletTransaction
	orders       map[string]RechargeOrder
	adminEmails  map[string]struct{}
	agreements   map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		wallets:      map[string]WalletSummary{},
		transactions: map[string][]WalletTransaction{},
		orders:       map[string]RechargeOrder{},
		adminEmails:  map[string]struct{}{},
		agreements:   map[string]string{},
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
	return out, nil
}

func (s *MemoryStore) SaveOrder(ctx context.Context, order RechargeOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[order.ID] = order
	return nil
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
	s.orders[orderID] = order

	wallet.BalanceFen += order.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	s.wallets[order.UserID] = wallet
	s.transactions[order.UserID] = append(s.transactions[order.UserID], WalletTransaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:      order.UserID,
		Kind:        "credit",
		AmountFen:   order.AmountFen,
		Description: description,
		CreatedUnix: time.Now().Unix(),
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

func (s *MemoryStore) RecordAgreementAcceptance(ctx context.Context, userID, key, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agreements[userID+":"+key] = version
	return nil
}

func (s *MemoryStore) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agreements[userID+":"+key] == version, nil
}
