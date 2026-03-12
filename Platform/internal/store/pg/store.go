package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"openclaw/platform/internal/service"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) SetBalance(userID string, balanceFen int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.pool.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, $2, 'CNY', now())
		on conflict (user_id) do update
		set balance_fen = excluded.balance_fen, updated_at = now()
	`, userID, balanceFen)
}

func (s *Store) GetWallet(ctx context.Context, userID string) (service.WalletSummary, error) {
	_, err := s.pool.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, 0, 'CNY', now())
		on conflict (user_id) do nothing
	`, userID)
	if err != nil {
		return service.WalletSummary{}, err
	}
	var wallet service.WalletSummary
	err = s.pool.QueryRow(ctx, `
		select user_id, balance_fen, currency, extract(epoch from updated_at)::bigint
		from wallet_accounts where user_id = $1
	`, userID).Scan(&wallet.UserID, &wallet.BalanceFen, &wallet.Currency, &wallet.UpdatedUnix)
	return wallet, err
}

func (s *Store) AppendTransaction(ctx context.Context, tx service.WalletTransaction) error {
	_, err := s.pool.Exec(ctx, `
		insert into wallet_transactions (id, user_id, kind, amount_fen, description, created_at)
		values ($1, $2, $3, $4, $5, to_timestamp($6))
	`, tx.ID, tx.UserID, tx.Kind, tx.AmountFen, tx.Description, tx.CreatedUnix)
	return err
}

func (s *Store) ListTransactions(ctx context.Context, userID string) ([]service.WalletTransaction, error) {
	rows, err := s.pool.Query(ctx, `
		select id, user_id, kind, amount_fen, description, extract(epoch from created_at)::bigint
		from wallet_transactions where user_id = $1 order by created_at desc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.WalletTransaction
	for rows.Next() {
		var item service.WalletTransaction
		if err := rows.Scan(&item.ID, &item.UserID, &item.Kind, &item.AmountFen, &item.Description, &item.CreatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveOrder(ctx context.Context, order service.RechargeOrder) error {
	_, err := s.pool.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, 0, 'CNY', now())
		on conflict (user_id) do nothing
	`, order.UserID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into recharge_orders (id, user_id, amount_fen, channel, provider, status, pay_url, external_order_id, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9), now())
	`, order.ID, order.UserID, order.AmountFen, order.Channel, order.Provider, order.Status, order.PayURL, order.ExternalID, order.CreatedUnix)
	return err
}

func (s *Store) GetOrder(ctx context.Context, userID, orderID string) (service.RechargeOrder, error) {
	var order service.RechargeOrder
	err := s.pool.QueryRow(ctx, `
		select id, user_id, amount_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''), extract(epoch from created_at)::bigint
		from recharge_orders where user_id = $1 and id = $2
	`, userID, orderID).Scan(&order.ID, &order.UserID, &order.AmountFen, &order.Channel, &order.Provider, &order.Status, &order.PayURL, &order.ExternalID, &order.CreatedUnix)
	return order, err
}

func (s *Store) FindOrderByID(ctx context.Context, orderID string) (service.RechargeOrder, error) {
	var order service.RechargeOrder
	err := s.pool.QueryRow(ctx, `
		select id, user_id, amount_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''), extract(epoch from created_at)::bigint
		from recharge_orders where id = $1
	`, orderID).Scan(&order.ID, &order.UserID, &order.AmountFen, &order.Channel, &order.Provider, &order.Status, &order.PayURL, &order.ExternalID, &order.CreatedUnix)
	return order, err
}

func (s *Store) MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (service.RechargeOrder, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.RechargeOrder{}, false, err
	}
	defer tx.Rollback(ctx)

	var order service.RechargeOrder
	err = tx.QueryRow(ctx, `
		select id, user_id, amount_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''), extract(epoch from created_at)::bigint
		from recharge_orders where id = $1 for update
	`, orderID).Scan(&order.ID, &order.UserID, &order.AmountFen, &order.Channel, &order.Provider, &order.Status, &order.PayURL, &order.ExternalID, &order.CreatedUnix)
	if err != nil {
		return service.RechargeOrder{}, false, err
	}
	if order.Status == "paid" {
		return order, false, nil
	}
	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	if _, err := tx.Exec(ctx, `
		update recharge_orders
		set status = 'paid', provider = $2, external_order_id = $3, updated_at = now()
		where id = $1
	`, orderID, provider, externalID); err != nil {
		return service.RechargeOrder{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.RechargeOrder{}, false, err
	}
	return order, true, nil
}

func (s *Store) FinalizeRechargeOrder(
	ctx context.Context,
	orderID, provider, externalID, description string,
) (service.RechargeOrder, service.WalletSummary, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	defer tx.Rollback(ctx)

	var order service.RechargeOrder
	err = tx.QueryRow(ctx, `
		select id, user_id, amount_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''), extract(epoch from created_at)::bigint
		from recharge_orders where id = $1 for update
	`, orderID).Scan(&order.ID, &order.UserID, &order.AmountFen, &order.Channel, &order.Provider, &order.Status, &order.PayURL, &order.ExternalID, &order.CreatedUnix)
	if err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}

	var balance int64
	if err := tx.QueryRow(ctx, `
		select balance_fen from wallet_accounts where user_id = $1 for update
	`, order.UserID).Scan(&balance); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}

	if order.Status == "paid" {
		return order, service.WalletSummary{
			UserID:      order.UserID,
			BalanceFen:  balance,
			Currency:    "CNY",
			UpdatedUnix: time.Now().Unix(),
		}, false, nil
	}

	newBalance := balance + order.AmountFen
	if _, err := tx.Exec(ctx, `
		update recharge_orders
		set status = 'paid', provider = $2, external_order_id = $3, updated_at = now()
		where id = $1
	`, orderID, provider, externalID); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		update wallet_accounts set balance_fen = $2, updated_at = now() where user_id = $1
	`, order.UserID, newBalance); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	txID := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `
		insert into wallet_transactions (id, user_id, kind, amount_fen, description, created_at)
		values ($1, $2, 'credit', $3, $4, now())
	`, txID, order.UserID, order.AmountFen, description); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}

	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	return order, service.WalletSummary{
		UserID:      order.UserID,
		BalanceFen:  newBalance,
		Currency:    "CNY",
		UpdatedUnix: time.Now().Unix(),
	}, true, nil
}

func (s *Store) Credit(ctx context.Context, userID string, amountFen int64, description string) (service.WalletSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, 0, 'CNY', now())
		on conflict (user_id) do nothing
	`, userID); err != nil {
		return service.WalletSummary{}, err
	}
	var balance int64
	if err := tx.QueryRow(ctx, `
		select balance_fen from wallet_accounts where user_id = $1 for update
	`, userID).Scan(&balance); err != nil {
		return service.WalletSummary{}, err
	}
	newBalance := balance + amountFen
	if _, err := tx.Exec(ctx, `
		update wallet_accounts set balance_fen = $2, updated_at = now() where user_id = $1
	`, userID, newBalance); err != nil {
		return service.WalletSummary{}, err
	}
	txID := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `
		insert into wallet_transactions (id, user_id, kind, amount_fen, description, created_at)
		values ($1, $2, 'credit', $3, $4, now())
	`, txID, userID, amountFen, description); err != nil {
		return service.WalletSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.WalletSummary{}, err
	}
	return service.WalletSummary{
		UserID:      userID,
		BalanceFen:  newBalance,
		Currency:    "CNY",
		UpdatedUnix: time.Now().Unix(),
	}, nil
}

func (s *Store) Debit(ctx context.Context, userID string, amountFen int64, description string) (service.WalletSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, err
	}
	defer tx.Rollback(ctx)

	var balance int64
	if err := tx.QueryRow(ctx, `
		select balance_fen from wallet_accounts where user_id = $1 for update
	`, userID).Scan(&balance); err != nil {
		return service.WalletSummary{}, err
	}
	if balance < amountFen {
		return service.WalletSummary{}, service.ErrInsufficientFunds
	}
	newBalance := balance - amountFen
	if _, err := tx.Exec(ctx, `
		update wallet_accounts set balance_fen = $2, updated_at = now() where user_id = $1
	`, userID, newBalance); err != nil {
		return service.WalletSummary{}, err
	}
	txID := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `
		insert into wallet_transactions (id, user_id, kind, amount_fen, description, created_at)
		values ($1, $2, 'debit', $3, $4, now())
	`, txID, userID, -amountFen, description); err != nil {
		return service.WalletSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.WalletSummary{}, err
	}
	return service.WalletSummary{
		UserID:      userID,
		BalanceFen:  newBalance,
		Currency:    "CNY",
		UpdatedUnix: time.Now().Unix(),
	}, nil
}

func (s *Store) UpsertAdminEmails(ctx context.Context, emails []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	normalized := make([]string, 0, len(emails))
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		normalized = append(normalized, email)
		if _, err := tx.Exec(ctx, `
			insert into admin_users (email, active, created_at)
			values ($1, true, now())
			on conflict (email) do update set active = true
		`, email); err != nil {
			return err
		}
	}
	if len(normalized) == 0 {
		if _, err := tx.Exec(ctx, `update admin_users set active = false`); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			update admin_users
			set active = false
			where lower(email) <> all($1)
		`, normalized); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1 from admin_users
			where active = true and (user_id = $1 or lower(email) = $2)
		)
	`, userID, email).Scan(&exists)
	return exists, err
}

func (s *Store) RecordAgreementAcceptance(ctx context.Context, userID, key, version string) error {
	_, err := s.pool.Exec(ctx, `
		insert into user_agreements (user_id, agreement_key, version, accepted_at)
		values ($1, $2, $3, now())
		on conflict (user_id, agreement_key) do update
		set version = excluded.version, accepted_at = now()
	`, userID, key, version)
	return err
}

func (s *Store) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1 from user_agreements
			where user_id = $1 and agreement_key = $2 and version = $3
		)
	`, userID, key, version).Scan(&exists)
	return exists, err
}
