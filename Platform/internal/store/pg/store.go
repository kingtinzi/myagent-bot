package pg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"openclaw/platform/internal/revisiontoken"
	"openclaw/platform/internal/service"
)

type Store struct {
	pool *pgxpool.Pool
}

func (s *Store) ensureUserProfile(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		insert into user_profiles (user_id, username, email, created_at, updated_at, last_seen_at)
		values ($1, '', '', now(), now(), now())
		on conflict (user_id) do nothing
	`, userID)
	return err
}

const (
	governanceRetentionLockKey int64 = 31001
	governanceNoticesLockKey   int64 = 31002
	governanceRiskLockKey      int64 = 31003
)

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
	_ = s.ensureUserProfile(ctx, userID)
	_, _ = s.pool.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, $2, 'CNY', now())
		on conflict (user_id) do update
		set balance_fen = excluded.balance_fen, updated_at = now()
	`, userID, balanceFen)
}

func (s *Store) GetWallet(ctx context.Context, userID string) (service.WalletSummary, error) {
	if err := s.ensureUserProfile(ctx, userID); err != nil {
		return service.WalletSummary{}, err
	}
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
	if err := s.ensureUserProfile(ctx, tx.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into wallet_transactions (
			id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
	`, tx.ID, tx.UserID, tx.Kind, tx.AmountFen, tx.Description, tx.ReferenceType, tx.ReferenceID, tx.PricingVersion, tx.CreatedUnix)
	return err
}

func (s *Store) ListTransactions(ctx context.Context, userID string) ([]service.WalletTransaction, error) {
	rows, err := s.pool.Query(ctx, `
		select id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version,
		       extract(epoch from created_at)::bigint
		from wallet_transactions
		where user_id = $1
		order by created_at desc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.WalletTransaction
	for rows.Next() {
		var item service.WalletTransaction
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Kind, &item.AmountFen, &item.Description,
			&item.ReferenceType, &item.ReferenceID, &item.PricingVersion, &item.CreatedUnix,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveOrder(ctx context.Context, order service.RechargeOrder) error {
	if err := s.ensureUserProfile(ctx, order.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, 0, 'CNY', now())
		on conflict (user_id) do nothing
	`, order.UserID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into recharge_orders (
			id, user_id, amount_fen, refunded_fen, channel, provider, status, pay_url, external_order_id,
			provider_status, pricing_version, agreement_versions, paid_at, last_checked_at, created_at, updated_at
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			case when $13 > 0 then to_timestamp($13) else null end,
			case when $14 > 0 then to_timestamp($14) else null end,
			to_timestamp($15), to_timestamp($16)
		)
		on conflict (id) do update set
			amount_fen = excluded.amount_fen,
			refunded_fen = excluded.refunded_fen,
			channel = excluded.channel,
			provider = excluded.provider,
			status = excluded.status,
			pay_url = excluded.pay_url,
			external_order_id = excluded.external_order_id,
			provider_status = excluded.provider_status,
			pricing_version = excluded.pricing_version,
			agreement_versions = excluded.agreement_versions,
			paid_at = excluded.paid_at,
			last_checked_at = excluded.last_checked_at,
			updated_at = excluded.updated_at
	`, order.ID, order.UserID, order.AmountFen, order.RefundedFen, order.Channel, order.Provider, order.Status, order.PayURL, order.ExternalID,
		order.ProviderStatus, order.PricingVersion, order.AgreementVersions, order.PaidUnix, order.LastCheckedUnix, order.CreatedUnix, order.UpdatedUnix)
	return err
}

func (s *Store) ListOrders(ctx context.Context) ([]service.RechargeOrder, error) {
	return s.ListOrdersFiltered(ctx, service.RechargeOrderFilter{})
}

func (s *Store) ListOrdersFiltered(ctx context.Context, filter service.RechargeOrderFilter) ([]service.RechargeOrder, error) {
	query, args := buildListOrdersQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (s *Store) GetOrder(ctx context.Context, userID, orderID string) (service.RechargeOrder, error) {
	var order service.RechargeOrder
	err := s.pool.QueryRow(ctx, `
		select id, user_id, amount_fen, refunded_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''),
		       coalesce(provider_status,''), coalesce(pricing_version,''), agreement_versions,
		       coalesce(extract(epoch from paid_at)::bigint, 0),
		       coalesce(extract(epoch from last_checked_at)::bigint, 0),
		       extract(epoch from created_at)::bigint,
		       extract(epoch from updated_at)::bigint
		from recharge_orders
		where user_id = $1 and id = $2
	`, userID, orderID).Scan(
		&order.ID, &order.UserID, &order.AmountFen, &order.RefundedFen, &order.Channel, &order.Provider, &order.Status,
		&order.PayURL, &order.ExternalID, &order.ProviderStatus, &order.PricingVersion, &order.AgreementVersions, &order.PaidUnix, &order.LastCheckedUnix, &order.CreatedUnix, &order.UpdatedUnix,
	)
	return order, err
}

func (s *Store) FindOrderByID(ctx context.Context, orderID string) (service.RechargeOrder, error) {
	var order service.RechargeOrder
	err := s.pool.QueryRow(ctx, `
		select id, user_id, amount_fen, refunded_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''),
		       coalesce(provider_status,''), coalesce(pricing_version,''), agreement_versions,
		       coalesce(extract(epoch from paid_at)::bigint, 0),
		       coalesce(extract(epoch from last_checked_at)::bigint, 0),
		       extract(epoch from created_at)::bigint,
		       extract(epoch from updated_at)::bigint
		from recharge_orders
		where id = $1
	`, orderID).Scan(
		&order.ID, &order.UserID, &order.AmountFen, &order.RefundedFen, &order.Channel, &order.Provider, &order.Status,
		&order.PayURL, &order.ExternalID, &order.ProviderStatus, &order.PricingVersion, &order.AgreementVersions, &order.PaidUnix, &order.LastCheckedUnix, &order.CreatedUnix, &order.UpdatedUnix,
	)
	return order, err
}

func (s *Store) MarkOrderPaid(ctx context.Context, orderID, provider, externalID string) (service.RechargeOrder, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.RechargeOrder{}, false, err
	}
	defer tx.Rollback(ctx)
	order, err := getOrderForUpdate(ctx, tx, orderID)
	if err != nil {
		return service.RechargeOrder{}, false, err
	}
	if order.Status == "paid" {
		return order, false, nil
	}
	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	order.PaidUnix = time.Now().Unix()
	order.UpdatedUnix = time.Now().Unix()
	if err := upsertOrderTx(ctx, tx, order); err != nil {
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
	order, err := getOrderForUpdate(ctx, tx, orderID)
	if err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	wallet, err := getWalletForUpdate(ctx, tx, order.UserID)
	if err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	if order.Status == "paid" {
		return order, wallet, false, nil
	}
	order.Status = "paid"
	order.Provider = provider
	order.ExternalID = externalID
	order.PaidUnix = time.Now().Unix()
	order.UpdatedUnix = time.Now().Unix()
	if err := upsertOrderTx(ctx, tx, order); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	wallet.BalanceFen += order.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	if err := upsertWalletTx(ctx, tx, wallet); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	if err := appendTransactionTx(ctx, tx, service.WalletTransaction{
		ID:            fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:        order.UserID,
		Kind:          "credit",
		AmountFen:     order.AmountFen,
		Description:   description,
		ReferenceType: "recharge_order",
		ReferenceID:   order.ID,
		CreatedUnix:   time.Now().Unix(),
	}); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.RechargeOrder{}, service.WalletSummary{}, false, err
	}
	return order, wallet, true, nil
}

func (s *Store) Credit(ctx context.Context, userID string, amountFen int64, description string) (service.WalletSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, err
	}
	defer tx.Rollback(ctx)
	wallet, err := getWalletForUpdate(ctx, tx, userID)
	if err != nil {
		return service.WalletSummary{}, err
	}
	wallet.BalanceFen += amountFen
	wallet.UpdatedUnix = time.Now().Unix()
	if err := upsertWalletTx(ctx, tx, wallet); err != nil {
		return service.WalletSummary{}, err
	}
	if err := appendTransactionTx(ctx, tx, service.WalletTransaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:      userID,
		Kind:        "credit",
		AmountFen:   amountFen,
		Description: description,
		CreatedUnix: time.Now().Unix(),
	}); err != nil {
		return service.WalletSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.WalletSummary{}, err
	}
	return wallet, nil
}

func (s *Store) Debit(ctx context.Context, userID string, amountFen int64, description string) (service.WalletSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, err
	}
	defer tx.Rollback(ctx)
	wallet, err := getWalletForUpdate(ctx, tx, userID)
	if err != nil {
		return service.WalletSummary{}, err
	}
	if wallet.BalanceFen < amountFen {
		return service.WalletSummary{}, service.ErrInsufficientFunds
	}
	wallet.BalanceFen -= amountFen
	wallet.UpdatedUnix = time.Now().Unix()
	if err := upsertWalletTx(ctx, tx, wallet); err != nil {
		return service.WalletSummary{}, err
	}
	if err := appendTransactionTx(ctx, tx, service.WalletTransaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		UserID:      userID,
		Kind:        "debit",
		AmountFen:   -amountFen,
		Description: description,
		CreatedUnix: time.Now().Unix(),
	}); err != nil {
		return service.WalletSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.WalletSummary{}, err
	}
	return wallet, nil
}

func (s *Store) UpsertAdminEmails(ctx context.Context, emails []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	normalized := make([]string, 0, len(emails))
	defaultCapabilities := service.DefaultAdminCapabilitiesForRole(service.AdminRoleSuperAdmin)
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		normalized = append(normalized, email)
		if _, err := tx.Exec(ctx, `
			insert into admin_users (email, active, role, capabilities, managed_by_seed, created_at, updated_at)
			values ($1, true, $2, $3, true, now(), now())
			on conflict (email) do update set active = true
			, role = case
				when coalesce(nullif(admin_users.role, ''), '') <> '' then admin_users.role
				else excluded.role
			end
			, capabilities = case
				when coalesce(array_length(admin_users.capabilities, 1), 0) > 0 then admin_users.capabilities
				else excluded.capabilities
			end
			, managed_by_seed = true
			, updated_at = now()
		`, email, service.AdminRoleSuperAdmin, defaultCapabilities); err != nil {
			return err
		}
	}
	if len(normalized) == 0 {
		if _, err := tx.Exec(ctx, `update admin_users set active = false, updated_at = now() where managed_by_seed = true`); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			update admin_users
			set active = false, updated_at = now()
			where managed_by_seed = true and lower(email) <> all($1)
		`, normalized); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) IsAdminUser(ctx context.Context, userID, email string) (bool, error) {
	operator, err := s.GetAdminOperator(ctx, userID, email)
	if err != nil {
		return false, err
	}
	return operator.Email != "" && operator.Active, nil
}

func (s *Store) GetAdminOperator(ctx context.Context, userID, email string) (service.AdminOperator, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	userID = strings.TrimSpace(userID)
	var item service.AdminOperator
	err := s.pool.QueryRow(ctx, `
		select coalesce(user_id,''), lower(email), coalesce(role,''), coalesce(capabilities, '{}'),
		       coalesce(managed_by_seed, false),
		       active, extract(epoch from created_at)::bigint,
		       coalesce(extract(epoch from updated_at)::bigint, 0)
		from admin_users
		where active = true and (
			($1 <> '' and user_id = $1) or
			($2 <> '' and lower(email) = $2 and ($1 = '' or coalesce(user_id, '') = '' or user_id = $1))
		)
		order by case
			when $1 <> '' and user_id = $1 then 0
			when coalesce(user_id, '') = '' then 1
			else 2
		end, email asc
		limit 1
	`, userID, email).Scan(&item.UserID, &item.Email, &item.Role, &item.Capabilities, &item.SeedManaged, &item.Active, &item.CreatedUnix, &item.UpdatedUnix)
	if err != nil {
		if err == pgx.ErrNoRows {
			return service.AdminOperator{}, nil
		}
		return service.AdminOperator{}, err
	}
	return service.NormalizeAdminOperator(item), nil
}

func (s *Store) ListAdminOperators(ctx context.Context) ([]service.AdminOperator, error) {
	rows, err := s.pool.Query(ctx, `
		select coalesce(user_id,''), lower(email), coalesce(role,''), coalesce(capabilities, '{}'),
		       coalesce(managed_by_seed, false),
		       active, extract(epoch from created_at)::bigint,
		       coalesce(extract(epoch from updated_at)::bigint, 0)
		from admin_users
		order by active desc, lower(email) asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]service.AdminOperator, 0)
	for rows.Next() {
		var item service.AdminOperator
		if err := rows.Scan(&item.UserID, &item.Email, &item.Role, &item.Capabilities, &item.SeedManaged, &item.Active, &item.CreatedUnix, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, service.NormalizeAdminOperator(item))
	}
	return items, rows.Err()
}

func (s *Store) SaveAdminOperator(ctx context.Context, operator service.AdminOperator) (service.AdminOperator, error) {
	operator = service.NormalizeAdminOperator(operator)
	err := s.pool.QueryRow(ctx, `
		insert into admin_users (email, user_id, active, role, capabilities, managed_by_seed, created_at, updated_at)
		values ($1, nullif($2,''), $3, $4, $5, false, to_timestamp($6), to_timestamp($7))
		on conflict (email) do update set
			user_id = case
				when excluded.user_id is not null then excluded.user_id
				else admin_users.user_id
			end,
			active = excluded.active,
			role = excluded.role,
			capabilities = excluded.capabilities,
			managed_by_seed = admin_users.managed_by_seed,
			updated_at = to_timestamp($7)
		returning coalesce(user_id,''), lower(email), coalesce(role,''), coalesce(capabilities, '{}'),
		          coalesce(managed_by_seed, false),
		          active, extract(epoch from created_at)::bigint,
		          coalesce(extract(epoch from updated_at)::bigint, 0)
	`, operator.Email, operator.UserID, operator.Active, operator.Role, operator.Capabilities, operator.CreatedUnix, operator.UpdatedUnix).
		Scan(&operator.UserID, &operator.Email, &operator.Role, &operator.Capabilities, &operator.SeedManaged, &operator.Active, &operator.CreatedUnix, &operator.UpdatedUnix)
	if err != nil {
		return service.AdminOperator{}, err
	}
	return service.NormalizeAdminOperator(operator), nil
}

func (s *Store) RecordAgreementAcceptance(ctx context.Context, acceptance service.AgreementAcceptance) error {
	if err := s.ensureUserProfile(ctx, acceptance.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into user_agreements (
			user_id, agreement_key, version, accepted_at, client_version, remote_addr, device_summary, content_checksum
		) values ($1, $2, $3, to_timestamp($4), $5, $6, $7, $8)
		on conflict (user_id, agreement_key, version) do update set
			accepted_at = excluded.accepted_at,
			client_version = excluded.client_version,
			remote_addr = excluded.remote_addr,
			device_summary = excluded.device_summary,
			content_checksum = excluded.content_checksum
	`, acceptance.UserID, acceptance.AgreementKey, acceptance.Version, acceptance.AcceptedUnix, acceptance.ClientVersion, acceptance.RemoteAddr, acceptance.DeviceSummary, acceptance.ContentChecksum)
	return err
}

func (s *Store) HasAgreementAcceptance(ctx context.Context, userID, key, version string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1 from user_agreements where user_id = $1 and agreement_key = $2 and version = $3
		)
	`, userID, key, version).Scan(&exists)
	return exists, err
}

func (s *Store) ListAgreementAcceptances(ctx context.Context, userID string) ([]service.AgreementAcceptance, error) {
	query := `
		select user_id, agreement_key, version, extract(epoch from accepted_at)::bigint,
		       coalesce(client_version,''), coalesce(remote_addr,''), coalesce(device_summary,''), coalesce(content_checksum,'')
		from user_agreements
	`
	args := []any{}
	if strings.TrimSpace(userID) != "" {
		query += ` where user_id = $1`
		args = append(args, userID)
	}
	query += ` order by accepted_at desc`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.AgreementAcceptance
	for rows.Next() {
		var item service.AgreementAcceptance
		if err := rows.Scan(&item.UserID, &item.AgreementKey, &item.Version, &item.AcceptedUnix, &item.ClientVersion, &item.RemoteAddr, &item.DeviceSummary, &item.ContentChecksum); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RecordChatUsage(ctx context.Context, usage service.ChatUsageRecord) error {
	if err := s.ensureUserProfile(ctx, usage.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into chat_usage_records (
			id, user_id, model_id, pricing_version, prompt_tokens, completion_tokens,
			charged_fen, fallback_applied, request_kind, agreement_versions, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, to_timestamp($11))
	`, usage.ID, usage.UserID, usage.ModelID, usage.PricingVersion, usage.PromptTokens, usage.CompletionTokens,
		usage.ChargedFen, usage.FallbackApplied, usage.RequestKind, usage.AgreementVersions, usage.CreatedUnix)
	return err
}

func (s *Store) ListChatUsageRecords(ctx context.Context, filter service.ChatUsageRecordFilter) ([]service.ChatUsageRecord, error) {
	query, args := buildListChatUsageRecordsQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChatUsageRecords(rows)
}

func (s *Store) BuildAdminDashboard(ctx context.Context, input service.AdminDashboardStoreInput) (service.AdminDashboard, error) {
	var dashboard service.AdminDashboard

	usersQuery, usersArgs := buildAdminDashboardUsersSummaryQuery(input)
	if err := s.pool.QueryRow(ctx, usersQuery, usersArgs...).Scan(
		&dashboard.Totals.Users,
		&dashboard.Totals.WalletBalanceFen,
		&dashboard.Recent.NewUsers7D,
	); err != nil {
		return service.AdminDashboard{}, err
	}

	ordersQuery, ordersArgs := buildAdminDashboardOrdersSummaryQuery(input)
	if err := s.pool.QueryRow(ctx, ordersQuery, ordersArgs...).Scan(
		&dashboard.Totals.PaidOrders,
		&dashboard.Recent.RechargeFen7D,
	); err != nil {
		return service.AdminDashboard{}, err
	}

	refundsQuery, refundsArgs := buildAdminDashboardRefundsSummaryQuery()
	if err := s.pool.QueryRow(ctx, refundsQuery, refundsArgs...).Scan(&dashboard.Totals.RefundPending); err != nil {
		return service.AdminDashboard{}, err
	}

	reportsQuery, reportsArgs := buildAdminDashboardInfringementSummaryQuery()
	if err := s.pool.QueryRow(ctx, reportsQuery, reportsArgs...).Scan(&dashboard.Totals.InfringementPending); err != nil {
		return service.AdminDashboard{}, err
	}

	usageQuery, usageArgs := buildAdminDashboardTopModelsQuery(input)
	rows, err := s.pool.Query(ctx, usageQuery, usageArgs...)
	if err != nil {
		return service.AdminDashboard{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item service.AdminDashboardModelStat
		if err := rows.Scan(
			&item.ModelID,
			&item.UsageCount,
			&item.ChargedFen,
			&item.PromptTokens,
			&item.CompletionTokens,
		); err != nil {
			return service.AdminDashboard{}, err
		}
		dashboard.Recent.ConsumptionFen7D += item.ChargedFen
		dashboard.TopModels = append(dashboard.TopModels, item)
	}
	if err := rows.Err(); err != nil {
		return service.AdminDashboard{}, err
	}

	return dashboard, nil
}

func (s *Store) UpsertUserIdentity(ctx context.Context, identity service.UserIdentity) error {
	_, err := s.pool.Exec(ctx, `
		insert into user_profiles (user_id, username, email, created_at, updated_at, last_seen_at)
		values ($1, $2, $3, to_timestamp($4), to_timestamp($5), to_timestamp($6))
		on conflict (user_id) do update set
			username = case
				when excluded.username <> '' then excluded.username
				else user_profiles.username
			end,
			email = case
				when excluded.email <> '' then excluded.email
				else user_profiles.email
			end,
			created_at = least(user_profiles.created_at, excluded.created_at),
			updated_at = greatest(user_profiles.updated_at, excluded.updated_at),
			last_seen_at = greatest(user_profiles.last_seen_at, excluded.last_seen_at)
	`, identity.UserID, identity.Username, identity.Email, identity.CreatedUnix, identity.UpdatedUnix, identity.LastSeenUnix)
	return err
}

func (s *Store) ApplyWalletAdjustment(ctx context.Context, txItem service.WalletTransaction) (service.WalletSummary, error) {
	if strings.TrimSpace(txItem.UserID) == "" || txItem.AmountFen == 0 {
		return service.WalletSummary{}, service.ErrInvalidAmount
	}
	if err := s.ensureUserProfile(ctx, txItem.UserID); err != nil {
		return service.WalletSummary{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, err
	}
	defer tx.Rollback(ctx)
	wallet, err := getWalletForUpdate(ctx, tx, txItem.UserID)
	if err != nil {
		return service.WalletSummary{}, err
	}
	if txItem.AmountFen < 0 && wallet.BalanceFen < -txItem.AmountFen {
		return service.WalletSummary{}, service.ErrInsufficientFunds
	}
	if wallet.Currency == "" {
		wallet.Currency = "CNY"
	}
	wallet.BalanceFen += txItem.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	if err := upsertWalletTx(ctx, tx, wallet); err != nil {
		return service.WalletSummary{}, err
	}
	if strings.TrimSpace(txItem.ID) == "" {
		txItem.ID = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(txItem.Kind) == "" {
		txItem.Kind = "credit"
		if txItem.AmountFen < 0 {
			txItem.Kind = "debit"
		}
	}
	if txItem.CreatedUnix == 0 {
		txItem.CreatedUnix = time.Now().Unix()
	}
	if err := appendTransactionTx(ctx, tx, txItem); err != nil {
		return service.WalletSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.WalletSummary{}, err
	}
	return wallet, nil
}

func (s *Store) ApplyAdminWalletMutation(
	ctx context.Context,
	txItem service.WalletTransaction,
	audit service.AdminAuditLog,
) (service.WalletSummary, bool, error) {
	if strings.TrimSpace(txItem.UserID) == "" || txItem.AmountFen == 0 {
		return service.WalletSummary{}, false, service.ErrInvalidAmount
	}
	if strings.TrimSpace(txItem.ReferenceType) == "" || strings.TrimSpace(txItem.ReferenceID) == "" {
		return service.WalletSummary{}, false, service.ErrInvalidRequestID
	}
	if err := s.ensureUserProfile(ctx, txItem.UserID); err != nil {
		return service.WalletSummary{}, false, err
	}

	dbtx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.WalletSummary{}, false, err
	}
	defer dbtx.Rollback(ctx)

	if existing, found, err := getWalletTransactionByReference(ctx, dbtx, txItem.ReferenceType, txItem.ReferenceID); err != nil {
		return service.WalletSummary{}, false, err
	} else if found {
		if !adminWalletMutationMatches(existing, txItem) {
			return service.WalletSummary{}, false, service.ErrIdempotencyConflict
		}
		wallet, err := getWalletForUpdate(ctx, dbtx, existing.UserID)
		if err != nil {
			return service.WalletSummary{}, false, err
		}
		return wallet, true, nil
	}

	wallet, err := getWalletForUpdate(ctx, dbtx, txItem.UserID)
	if err != nil {
		return service.WalletSummary{}, false, err
	}
	if txItem.AmountFen < 0 && wallet.BalanceFen < -txItem.AmountFen {
		return service.WalletSummary{}, false, service.ErrInsufficientFunds
	}
	if wallet.Currency == "" {
		wallet.Currency = "CNY"
	}
	wallet.BalanceFen += txItem.AmountFen
	wallet.UpdatedUnix = time.Now().Unix()
	if err := upsertWalletTx(ctx, dbtx, wallet); err != nil {
		return service.WalletSummary{}, false, err
	}
	if strings.TrimSpace(txItem.ID) == "" {
		txItem.ID = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(txItem.Kind) == "" {
		txItem.Kind = "credit"
		if txItem.AmountFen < 0 {
			txItem.Kind = "debit"
		}
	}
	if txItem.CreatedUnix == 0 {
		txItem.CreatedUnix = time.Now().Unix()
	}
	inserted, err := appendTransactionTxIdempotent(ctx, dbtx, txItem)
	if err != nil {
		return service.WalletSummary{}, false, err
	}
	if !inserted {
		if err := dbtx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return service.WalletSummary{}, false, err
		}
		return s.loadAdminWalletMutationReplay(ctx, txItem)
	}
	if strings.TrimSpace(audit.ID) == "" {
		audit.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}
	if audit.CreatedUnix == 0 {
		audit.CreatedUnix = time.Now().Unix()
	}
	if err := appendAuditLogTx(ctx, dbtx, audit); err != nil {
		return service.WalletSummary{}, false, err
	}
	if err := dbtx.Commit(ctx); err != nil {
		return service.WalletSummary{}, false, err
	}
	return wallet, false, nil
}

func (s *Store) loadAdminWalletMutationReplay(
	ctx context.Context,
	txItem service.WalletTransaction,
) (service.WalletSummary, bool, error) {
	existing, found, err := getWalletTransactionByReference(ctx, s.pool, txItem.ReferenceType, txItem.ReferenceID)
	if err != nil {
		return service.WalletSummary{}, false, err
	}
	if !found {
		return service.WalletSummary{}, false, service.ErrIdempotencyConflict
	}
	if !adminWalletMutationMatches(existing, txItem) {
		return service.WalletSummary{}, false, service.ErrIdempotencyConflict
	}
	wallet, err := s.GetWallet(ctx, existing.UserID)
	if err != nil {
		return service.WalletSummary{}, false, err
	}
	return wallet, true, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]service.UserSummary, error) {
	return s.ListUsersFiltered(ctx, service.UserSummaryFilter{})
}

func (s *Store) ListUsersFiltered(ctx context.Context, filter service.UserSummaryFilter) ([]service.UserSummary, error) {
	query, args := buildListUsersQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) ListWalletAdjustments(ctx context.Context) ([]service.WalletTransaction, error) {
	return s.ListWalletAdjustmentsFiltered(ctx, service.WalletAdjustmentFilter{})
}

func (s *Store) ListWalletAdjustmentsFiltered(ctx context.Context, filter service.WalletAdjustmentFilter) ([]service.WalletTransaction, error) {
	query, args := buildListWalletAdjustmentsQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWalletTransactions(rows)
}

func (s *Store) AppendAuditLog(ctx context.Context, entry service.AdminAuditLog) error {
	return appendAuditLogTx(ctx, s.pool, entry)
}

func (s *Store) ListAuditLogs(ctx context.Context, filter service.AuditLogFilter) ([]service.AdminAuditLog, error) {
	query, args := buildListAuditLogsQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.AdminAuditLog
	for rows.Next() {
		var item service.AdminAuditLog
		if err := rows.Scan(&item.ID, &item.ActorUserID, &item.ActorEmail, &item.Action, &item.TargetType, &item.TargetID, &item.RiskLevel, &item.Detail, &item.CreatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateRefundRequest(ctx context.Context, request service.RefundRequest) error {
	if err := s.ensureUserProfile(ctx, request.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into refund_requests (
			id, user_id, order_id, amount_fen, reason, status, review_note, reviewed_by, refund_provider,
			external_refund_id, external_status, failure_reason, settled_at, created_at, updated_at
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12,
			case when $13 > 0 then to_timestamp($13) else null end,
			to_timestamp($14), to_timestamp($15)
		)
	`, request.ID, request.UserID, request.OrderID, request.AmountFen, request.Reason, request.Status, request.ReviewNote, request.ReviewedBy, request.RefundProvider, request.ExternalRefundID, request.ExternalStatus, request.FailureReason, request.SettledUnix, request.CreatedUnix, request.UpdatedUnix)
	return err
}

func (s *Store) SaveRefundRequest(ctx context.Context, request service.RefundRequest) error {
	_, err := s.pool.Exec(ctx, `
		update refund_requests set
			status = $2, review_note = $3, reviewed_by = $4, refund_provider = $5,
			external_refund_id = $6, external_status = $7, failure_reason = $8,
			settled_at = case when $9 > 0 then to_timestamp($9) else null end,
			updated_at = to_timestamp($10)
		where id = $1
	`, request.ID, request.Status, request.ReviewNote, request.ReviewedBy, request.RefundProvider, request.ExternalRefundID, request.ExternalStatus, request.FailureReason, request.SettledUnix, request.UpdatedUnix)
	return err
}

func (s *Store) GetRefundRequest(ctx context.Context, requestID string) (service.RefundRequest, error) {
	var item service.RefundRequest
	err := s.pool.QueryRow(ctx, `
		select id, user_id, order_id, amount_fen, reason, status, review_note, reviewed_by, refund_provider,
		       coalesce(external_refund_id,''), coalesce(external_status,''), coalesce(failure_reason,''),
		       coalesce(extract(epoch from settled_at)::bigint, 0),
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from refund_requests where id = $1
	`, requestID).Scan(&item.ID, &item.UserID, &item.OrderID, &item.AmountFen, &item.Reason, &item.Status, &item.ReviewNote, &item.ReviewedBy, &item.RefundProvider, &item.ExternalRefundID, &item.ExternalStatus, &item.FailureReason, &item.SettledUnix, &item.CreatedUnix, &item.UpdatedUnix)
	return item, err
}

func (s *Store) ListRefundRequests(ctx context.Context, userID string) ([]service.RefundRequest, error) {
	return s.ListRefundRequestsFiltered(ctx, service.RefundRequestFilter{UserID: userID})
}

func (s *Store) ListRefundRequestsFiltered(ctx context.Context, filter service.RefundRequestFilter) ([]service.RefundRequest, error) {
	query, args := buildListRefundRequestsQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRefundRequests(rows)
}

func (s *Store) ApplyRefundDecision(ctx context.Context, requestID string, input service.RefundDecisionInput, updatedUnix int64) (service.RefundRequest, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return service.RefundRequest{}, err
	}
	defer tx.Rollback(ctx)

	var request service.RefundRequest
	err = tx.QueryRow(ctx, `
		select id, user_id, order_id, amount_fen, reason, status, review_note, reviewed_by, refund_provider,
		       coalesce(external_refund_id,''), coalesce(external_status,''), coalesce(failure_reason,''),
		       coalesce(extract(epoch from settled_at)::bigint, 0),
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from refund_requests
		where id = $1
		for update
	`, requestID).Scan(&request.ID, &request.UserID, &request.OrderID, &request.AmountFen, &request.Reason, &request.Status, &request.ReviewNote, &request.ReviewedBy, &request.RefundProvider, &request.ExternalRefundID, &request.ExternalStatus, &request.FailureReason, &request.SettledUnix, &request.CreatedUnix, &request.UpdatedUnix)
	if err != nil {
		return service.RefundRequest{}, err
	}
	if request.Status != "pending" && request.Status != "approved_pending_payout" {
		return service.RefundRequest{}, fmt.Errorf("%w: refund request %s is already %s", service.ErrRefundNotAllowed, requestID, request.Status)
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "approved" {
		status = "refunded"
	}
	switch status {
	case "refunded", "rejected", "refund_failed", "approved_pending_payout":
	default:
		return service.RefundRequest{}, fmt.Errorf("%w: unsupported refund decision status %s", service.ErrRefundNotAllowed, status)
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
		order, err := getOrderForUpdate(ctx, tx, request.OrderID)
		if err != nil {
			return service.RefundRequest{}, err
		}
		remainingRefundable := order.AmountFen - order.RefundedFen
		if remainingRefundable <= 0 || request.AmountFen > remainingRefundable {
			return service.RefundRequest{}, fmt.Errorf("%w: order %s only has %d fen refundable", service.ErrRefundNotAllowed, request.OrderID, remainingRefundable)
		}
		wallet, err := getWalletForUpdate(ctx, tx, request.UserID)
		if err != nil {
			return service.RefundRequest{}, err
		}
		if !allowNegativeBalance && wallet.BalanceFen < request.AmountFen {
			return service.RefundRequest{}, fmt.Errorf("%w: wallet balance %d fen is lower than requested refund %d fen", service.ErrRefundNotAllowed, wallet.BalanceFen, request.AmountFen)
		}
		wallet.BalanceFen -= request.AmountFen
		wallet.UpdatedUnix = updatedUnix
		if err := upsertWalletTx(ctx, tx, wallet); err != nil {
			return service.RefundRequest{}, err
		}
		order.RefundedFen += request.AmountFen
		if order.RefundedFen > order.AmountFen {
			order.RefundedFen = order.AmountFen
		}
		if order.RefundedFen == order.AmountFen {
			order.Status = "refunded"
		}
		order.UpdatedUnix = updatedUnix
		if err := upsertOrderTx(ctx, tx, order); err != nil {
			return service.RefundRequest{}, err
		}
		if err := appendTransactionTx(ctx, tx, service.WalletTransaction{
			ID:            fmt.Sprintf("tx_%d", time.Now().UnixNano()),
			UserID:        request.UserID,
			Kind:          "refund",
			AmountFen:     -request.AmountFen,
			Description:   "refund payout",
			ReferenceType: "refund_request",
			ReferenceID:   request.ID,
			CreatedUnix:   updatedUnix,
		}); err != nil {
			return service.RefundRequest{}, err
		}
		request.SettledUnix = updatedUnix
	}

	_, err = tx.Exec(ctx, `
		update refund_requests
		set status = $2, review_note = $3, reviewed_by = $4, refund_provider = $5,
		    external_refund_id = $6, external_status = $7, failure_reason = $8,
		    settled_at = case when $9 > 0 then to_timestamp($9) else null end,
		    updated_at = to_timestamp($10)
		where id = $1
	`, request.ID, request.Status, request.ReviewNote, request.ReviewedBy, request.RefundProvider, request.ExternalRefundID, request.ExternalStatus, request.FailureReason, request.SettledUnix, request.UpdatedUnix)
	if err != nil {
		return service.RefundRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.RefundRequest{}, err
	}
	return request, nil
}

func (s *Store) CreateInfringementReport(ctx context.Context, report service.InfringementReport) error {
	if err := s.ensureUserProfile(ctx, report.UserID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		insert into infringement_reports (
			id, user_id, subject, description, evidence_urls, status, resolution, reviewed_by, created_at, updated_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9), to_timestamp($10))
	`, report.ID, report.UserID, report.Subject, report.Description, report.EvidenceURLs, report.Status, report.Resolution, report.ReviewedBy, report.CreatedUnix, report.UpdatedUnix)
	return err
}

func (s *Store) SaveInfringementReport(ctx context.Context, report service.InfringementReport) error {
	_, err := s.pool.Exec(ctx, `
		update infringement_reports
		set status = $2, resolution = $3, reviewed_by = $4, updated_at = to_timestamp($5)
		where id = $1
	`, report.ID, report.Status, report.Resolution, report.ReviewedBy, report.UpdatedUnix)
	return err
}

func (s *Store) GetInfringementReport(ctx context.Context, reportID string) (service.InfringementReport, error) {
	var item service.InfringementReport
	err := s.pool.QueryRow(ctx, `
		select id, user_id, subject, description, evidence_urls, status, resolution, reviewed_by,
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from infringement_reports where id = $1
	`, reportID).Scan(&item.ID, &item.UserID, &item.Subject, &item.Description, &item.EvidenceURLs, &item.Status, &item.Resolution, &item.ReviewedBy, &item.CreatedUnix, &item.UpdatedUnix)
	return item, err
}

func (s *Store) ListInfringementReports(ctx context.Context, userID string) ([]service.InfringementReport, error) {
	return s.ListInfringementReportsFiltered(ctx, service.InfringementReportFilter{UserID: userID})
}

func (s *Store) ListInfringementReportsFiltered(ctx context.Context, filter service.InfringementReportFilter) ([]service.InfringementReport, error) {
	query, args := buildListInfringementReportsQuery(filter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInfringementReports(rows)
}

func buildListAuditLogsQuery(filter service.AuditLogFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.Action != "" {
		clauses = append(clauses, "action = "+next(filter.Action))
	}
	if filter.TargetType != "" {
		clauses = append(clauses, "target_type = "+next(filter.TargetType))
	}
	if filter.TargetID != "" {
		clauses = append(clauses, "target_id = "+next(filter.TargetID))
	}
	if filter.ActorUserID != "" {
		clauses = append(clauses, "actor_user_id = "+next(filter.ActorUserID))
	}
	if filter.RiskLevel != "" {
		clauses = append(clauses, "risk_level = "+next(strings.TrimSpace(filter.RiskLevel)))
	}
	if filter.SinceUnix > 0 {
		clauses = append(clauses, "created_at >= to_timestamp("+next(filter.SinceUnix)+")")
	}
	if filter.UntilUnix > 0 {
		clauses = append(clauses, "created_at <= to_timestamp("+next(filter.UntilUnix)+")")
	}
	query := strings.Builder{}
	query.WriteString(`
		select id, actor_user_id, actor_email, action, target_type, target_id, risk_level, detail,
		       extract(epoch from created_at)::bigint
		from admin_audit_logs
	`)
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListOrdersQuery(filter service.RechargeOrderFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select o.id, o.user_id, coalesce(p.user_no, 0), coalesce(nullif(p.username, ''), ''), o.amount_fen, o.refunded_fen, o.channel, o.provider, o.status, coalesce(o.pay_url,''), coalesce(o.external_order_id,''),
		       coalesce(o.provider_status,''), coalesce(o.pricing_version,''), o.agreement_versions,
		       coalesce(extract(epoch from o.paid_at)::bigint, 0),
		       coalesce(extract(epoch from o.last_checked_at)::bigint, 0),
		       extract(epoch from o.created_at)::bigint,
		       extract(epoch from o.updated_at)::bigint
		from recharge_orders o
		left join user_profiles p on p.user_id = o.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("o.user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.UserKeyword); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			args = append(args, parsed)
			userNoArg := len(args)
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(p.user_no = $%d or lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or o.user_id = $%d)", userNoArg, usernameArg, emailArg, userIDArg))
		} else {
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or o.user_id = $%d)", usernameArg, emailArg, userIDArg))
		}
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(o.status) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Provider); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(o.provider) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "o.created_at desc, o.id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListUsersQuery(filter service.UserSummaryFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		with user_registry as (
			select user_id from user_profiles
			union
			select user_id from wallet_accounts
		)
		select u.user_id,
		       coalesce(p.user_no, 0),
		       coalesce(nullif(p.username, ''), ''),
		       coalesce(nullif(p.email, ''), ''),
		       coalesce(extract(epoch from p.created_at)::bigint, 0),
		       coalesce(extract(epoch from p.last_seen_at)::bigint, 0),
		       coalesce(w.balance_fen, 0),
		       coalesce(nullif(w.currency, ''), 'CNY'),
		       greatest(
		           coalesce(extract(epoch from p.updated_at)::bigint, 0),
		           coalesce(extract(epoch from p.last_seen_at)::bigint, 0),
		           coalesce(extract(epoch from w.updated_at)::bigint, 0)
		       ) as updated_unix,
		       coalesce(o.order_count, 0), coalesce(r.refund_count, 0),
		       coalesce(o.last_order_unix, 0), coalesce(r.last_refund_unix, 0)
		from user_registry u
		left join user_profiles p on p.user_id = u.user_id
		left join wallet_accounts w on w.user_id = u.user_id
		left join (
		  select user_id, count(*)::int as order_count, max(extract(epoch from created_at)::bigint) as last_order_unix
		  from recharge_orders group by user_id
		) o on o.user_id = u.user_id
		left join (
		  select user_id, count(*)::int as refund_count, max(extract(epoch from created_at)::bigint) as last_refund_unix
		  from refund_requests group by user_id
		) r on r.user_id = u.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("u.user_id = $%d", len(args)))
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Email)); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("lower(coalesce(p.email,'')) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			args = append(args, parsed)
			userNoArg := len(args)
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(p.user_no = $%d or lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or u.user_id = $%d)", userNoArg, usernameArg, emailArg, userIDArg))
		} else {
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or u.user_id = $%d)", usernameArg, emailArg, userIDArg))
		}
	}
	appendListClauses(&query, &args, clauses, "updated_unix desc, coalesce(nullif(p.user_no, 0), 9223372036854775807) asc, u.user_id asc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListWalletAdjustmentsQuery(filter service.WalletAdjustmentFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select w.id, w.user_id, coalesce(p.user_no, 0), coalesce(nullif(p.username, ''), ''), w.kind, w.amount_fen, w.description, w.reference_type, w.reference_id, w.pricing_version,
		       extract(epoch from w.created_at)::bigint
		from wallet_transactions w
		left join user_profiles p on p.user_id = w.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("w.user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.UserKeyword); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			args = append(args, parsed)
			userNoArg := len(args)
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(p.user_no = $%d or lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or w.user_id = $%d)", userNoArg, usernameArg, emailArg, userIDArg))
		} else {
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or w.user_id = $%d)", usernameArg, emailArg, userIDArg))
		}
	}
	if value := strings.TrimSpace(filter.Kind); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(w.kind) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.ReferenceType); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(coalesce(w.reference_type,'')) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "w.created_at desc, w.id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListRefundRequestsQuery(filter service.RefundRequestFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select r.id, r.user_id, coalesce(p.user_no, 0), coalesce(nullif(p.username, ''), ''), r.order_id, r.amount_fen, r.reason, r.status, r.review_note, r.reviewed_by, r.refund_provider,
		       coalesce(r.external_refund_id,''), coalesce(r.external_status,''), coalesce(r.failure_reason,''),
		       coalesce(extract(epoch from r.settled_at)::bigint, 0),
		       extract(epoch from r.created_at)::bigint, extract(epoch from r.updated_at)::bigint
		from refund_requests r
		left join user_profiles p on p.user_id = r.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("r.user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.UserKeyword); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			args = append(args, parsed)
			userNoArg := len(args)
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(p.user_no = $%d or lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or r.user_id = $%d)", userNoArg, usernameArg, emailArg, userIDArg))
		} else {
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or r.user_id = $%d)", usernameArg, emailArg, userIDArg))
		}
	}
	if value := strings.TrimSpace(filter.OrderID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("r.order_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(r.status) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "r.created_at desc, r.id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListInfringementReportsQuery(filter service.InfringementReportFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select i.id, i.user_id, coalesce(p.user_no, 0), coalesce(nullif(p.username, ''), ''), i.subject, i.description, i.evidence_urls, i.status, i.resolution, i.reviewed_by,
		       extract(epoch from i.created_at)::bigint, extract(epoch from i.updated_at)::bigint
		from infringement_reports i
		left join user_profiles p on p.user_id = i.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("i.user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.UserKeyword); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			args = append(args, parsed)
			userNoArg := len(args)
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(p.user_no = $%d or lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or i.user_id = $%d)", userNoArg, usernameArg, emailArg, userIDArg))
		} else {
			args = append(args, strings.ToLower(value))
			usernameArg := len(args)
			args = append(args, strings.ToLower(value))
			emailArg := len(args)
			args = append(args, value)
			userIDArg := len(args)
			clauses = append(clauses, fmt.Sprintf("(lower(coalesce(p.username,'')) = $%d or lower(coalesce(p.email,'')) = $%d or i.user_id = $%d)", usernameArg, emailArg, userIDArg))
		}
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(i.status) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.ReviewedBy); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("i.reviewed_by = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "i.created_at desc, i.id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListChatUsageRecordsQuery(filter service.ChatUsageRecordFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select id, user_id, model_id, pricing_version, prompt_tokens, completion_tokens,
		       charged_fen, fallback_applied, request_kind, agreement_versions,
		       extract(epoch from created_at)::bigint
		from chat_usage_records
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.ModelID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("model_id = $%d", len(args)))
	}
	if filter.SinceUnix > 0 {
		args = append(args, filter.SinceUnix)
		clauses = append(clauses, fmt.Sprintf("created_at >= to_timestamp($%d)", len(args)))
	}
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildAdminDashboardUsersSummaryQuery(input service.AdminDashboardStoreInput) (string, []any) {
	return `
		with user_registry as (
			select user_id from user_profiles
			union
			select user_id from wallet_accounts
		)
		select
			count(*)::int as user_count,
			coalesce(sum(coalesce(w.balance_fen, 0)), 0)::bigint as wallet_balance_fen,
			count(*) filter (
				where coalesce(extract(epoch from p.created_at)::bigint, 0) >= $3
			)::int as new_users_7d
		from user_registry u
		left join user_profiles p on p.user_id = u.user_id
		left join wallet_accounts w on w.user_id = u.user_id
		where (cardinality($1::text[]) = 0 or u.user_id <> all($1))
		  and (cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2))
	`, []any{input.ExcludedAdminUserIDs, input.ExcludedAdminEmails, input.SinceUnix}
}

func buildAdminDashboardOrdersSummaryQuery(input service.AdminDashboardStoreInput) (string, []any) {
	return `
		select
			count(*)::int as paid_orders,
			coalesce(sum(
				case
					when coalesce(extract(epoch from r.created_at)::bigint, 0) >= $3 then r.amount_fen
					else 0
				end
			), 0)::bigint as recharge_fen_7d
		from recharge_orders r
		left join user_profiles p on p.user_id = r.user_id
		where (cardinality($1::text[]) = 0 or r.user_id <> all($1))
		  and (cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2))
		  and (
			lower(coalesce(r.status, '')) in ('paid', 'refunded')
			or coalesce(extract(epoch from r.paid_at)::bigint, 0) > 0
		  )
	`, []any{input.ExcludedAdminUserIDs, input.ExcludedAdminEmails, input.SinceUnix}
}

func buildAdminDashboardRefundsSummaryQuery() (string, []any) {
	return `
		select count(*)::int
		from refund_requests
		where lower(coalesce(status, '')) in ('pending', 'approved_pending_payout', 'refund_failed')
	`, nil
}

func buildAdminDashboardInfringementSummaryQuery() (string, []any) {
	return `
		select count(*)::int
		from infringement_reports
		where lower(coalesce(status, '')) in ('', 'pending', 'reviewing')
	`, nil
}

func buildAdminDashboardTopModelsQuery(input service.AdminDashboardStoreInput) (string, []any) {
	return `
		select
			u.model_id,
			count(*)::int as usage_count,
			coalesce(sum(u.charged_fen), 0)::bigint as charged_fen,
			coalesce(sum(u.prompt_tokens), 0)::int as prompt_tokens,
			coalesce(sum(u.completion_tokens), 0)::int as completion_tokens
		from chat_usage_records u
		left join user_profiles p on p.user_id = u.user_id
		where (cardinality($1::text[]) = 0 or u.user_id <> all($1))
		  and (cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2))
		  and u.created_at >= to_timestamp($3)
		group by u.model_id
		order by charged_fen desc, usage_count desc, model_id asc
	`, []any{input.ExcludedAdminUserIDs, input.ExcludedAdminEmails, input.SinceUnix}
}

func appendListClauses(query *strings.Builder, args *[]any, clauses []string, orderBy string, limit, offset int) {
	if len(clauses) > 0 {
		query.WriteString(" where ")
		query.WriteString(strings.Join(clauses, " and "))
	}
	query.WriteString(" order by ")
	query.WriteString(orderBy)
	if limit > 0 {
		*args = append(*args, limit)
		query.WriteString(fmt.Sprintf(" limit $%d", len(*args)))
	}
	if offset > 0 {
		*args = append(*args, offset)
		query.WriteString(fmt.Sprintf(" offset $%d", len(*args)))
	}
}

func scanUsers(rows pgx.Rows) ([]service.UserSummary, error) {
	var items []service.UserSummary
	for rows.Next() {
		var item service.UserSummary
		if err := rows.Scan(&item.UserID, &item.UserNo, &item.Username, &item.Email, &item.CreatedUnix, &item.LastSeenUnix, &item.BalanceFen, &item.Currency, &item.UpdatedUnix, &item.OrderCount, &item.RefundCount, &item.LastOrderUnix, &item.LastRefundUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWalletTransactions(rows pgx.Rows) ([]service.WalletTransaction, error) {
	var items []service.WalletTransaction
	for rows.Next() {
		var item service.WalletTransaction
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserNo, &item.Username, &item.Kind, &item.AmountFen, &item.Description, &item.ReferenceType, &item.ReferenceID, &item.PricingVersion, &item.CreatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanChatUsageRecords(rows pgx.Rows) ([]service.ChatUsageRecord, error) {
	var items []service.ChatUsageRecord
	for rows.Next() {
		var item service.ChatUsageRecord
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.ModelID,
			&item.PricingVersion,
			&item.PromptTokens,
			&item.CompletionTokens,
			&item.ChargedFen,
			&item.FallbackApplied,
			&item.RequestKind,
			&item.AgreementVersions,
			&item.CreatedUnix,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanRefundRequests(rows pgx.Rows) ([]service.RefundRequest, error) {
	var items []service.RefundRequest
	for rows.Next() {
		var item service.RefundRequest
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserNo, &item.Username, &item.OrderID, &item.AmountFen, &item.Reason, &item.Status, &item.ReviewNote, &item.ReviewedBy, &item.RefundProvider, &item.ExternalRefundID, &item.ExternalStatus, &item.FailureReason, &item.SettledUnix, &item.CreatedUnix, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanInfringementReports(rows pgx.Rows) ([]service.InfringementReport, error) {
	var items []service.InfringementReport
	for rows.Next() {
		var item service.InfringementReport
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserNo, &item.Username, &item.Subject, &item.Description, &item.EvidenceURLs, &item.Status, &item.Resolution, &item.ReviewedBy, &item.CreatedUnix, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListDataRetentionPolicies(ctx context.Context) ([]service.DataRetentionPolicy, error) {
	rows, err := s.pool.Query(ctx, `
		select data_domain, retention_days, purge_mode, description, enabled, extract(epoch from updated_at)::bigint
		from data_retention_policies
		order by data_domain asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.DataRetentionPolicy
	for rows.Next() {
		var item service.DataRetentionPolicy
		if err := rows.Scan(&item.DataDomain, &item.RetentionDays, &item.PurgeMode, &item.Description, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveDataRetentionPolicies(ctx context.Context, policies []service.DataRetentionPolicy) error {
	return s.SaveDataRetentionPoliciesWithRevision(ctx, "", policies)
}

func (s *Store) SaveDataRetentionPoliciesWithRevision(ctx context.Context, expectedRevision string, policies []service.DataRetentionPolicy) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGovernanceCollection(ctx, tx, governanceRetentionLockKey); err != nil {
		return err
	}
	if err := ensureGovernanceRevisionTx(ctx, tx, expectedRevision, listDataRetentionPoliciesTx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from data_retention_policies`); err != nil {
		return err
	}
	for _, item := range policies {
		if _, err := tx.Exec(ctx, `
			insert into data_retention_policies (data_domain, retention_days, purge_mode, description, enabled, updated_at)
			values ($1, $2, $3, $4, $5, to_timestamp($6))
		`, item.DataDomain, item.RetentionDays, item.PurgeMode, item.Description, item.Enabled, item.UpdatedUnix); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListSystemNotices(ctx context.Context) ([]service.SystemNotice, error) {
	rows, err := s.pool.Query(ctx, `
		select id, title, body, severity, enabled, extract(epoch from updated_at)::bigint
		from system_notices
		order by id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.SystemNotice
	for rows.Next() {
		var item service.SystemNotice
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Severity, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveSystemNotices(ctx context.Context, notices []service.SystemNotice) error {
	return s.SaveSystemNoticesWithRevision(ctx, "", notices)
}

func (s *Store) SaveSystemNoticesWithRevision(ctx context.Context, expectedRevision string, notices []service.SystemNotice) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGovernanceCollection(ctx, tx, governanceNoticesLockKey); err != nil {
		return err
	}
	if err := ensureGovernanceRevisionTx(ctx, tx, expectedRevision, listSystemNoticesTx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from system_notices`); err != nil {
		return err
	}
	for _, item := range notices {
		if _, err := tx.Exec(ctx, `
			insert into system_notices (id, title, body, severity, enabled, updated_at)
			values ($1, $2, $3, $4, $5, to_timestamp($6))
		`, item.ID, item.Title, item.Body, item.Severity, item.Enabled, item.UpdatedUnix); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListRiskRules(ctx context.Context) ([]service.RiskRule, error) {
	rows, err := s.pool.Query(ctx, `
		select key, name, description, enabled, extract(epoch from updated_at)::bigint
		from risk_rules
		order by key asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.RiskRule
	for rows.Next() {
		var item service.RiskRule
		if err := rows.Scan(&item.Key, &item.Name, &item.Description, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveRiskRules(ctx context.Context, rules []service.RiskRule) error {
	return s.SaveRiskRulesWithRevision(ctx, "", rules)
}

func (s *Store) SaveRiskRulesWithRevision(ctx context.Context, expectedRevision string, rules []service.RiskRule) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGovernanceCollection(ctx, tx, governanceRiskLockKey); err != nil {
		return err
	}
	if err := ensureGovernanceRevisionTx(ctx, tx, expectedRevision, listRiskRulesTx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from risk_rules`); err != nil {
		return err
	}
	for _, item := range rules {
		if _, err := tx.Exec(ctx, `
			insert into risk_rules (key, name, description, enabled, updated_at)
			values ($1, $2, $3, $4, to_timestamp($5))
		`, item.Key, item.Name, item.Description, item.Enabled, item.UpdatedUnix); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func lockGovernanceCollection(ctx context.Context, tx pgx.Tx, key int64) error {
	_, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, key)
	return err
}

func ensureGovernanceRevisionTx[T any](ctx context.Context, tx pgx.Tx, expectedRevision string, listFn func(context.Context, pgx.Tx) ([]T, error)) error {
	expectedRevision = strings.TrimSpace(expectedRevision)
	if expectedRevision == "" {
		return nil
	}
	current, err := listFn(ctx, tx)
	if err != nil {
		return err
	}
	revision, err := revisiontoken.ForPayload(current)
	if err != nil {
		return err
	}
	if revisiontoken.Matches(expectedRevision, revision) {
		return nil
	}
	return service.ErrRevisionConflict
}

func listDataRetentionPoliciesTx(ctx context.Context, tx pgx.Tx) ([]service.DataRetentionPolicy, error) {
	rows, err := tx.Query(ctx, `
		select data_domain, retention_days, purge_mode, description, enabled, extract(epoch from updated_at)::bigint
		from data_retention_policies
		order by data_domain asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.DataRetentionPolicy
	for rows.Next() {
		var item service.DataRetentionPolicy
		if err := rows.Scan(&item.DataDomain, &item.RetentionDays, &item.PurgeMode, &item.Description, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func listSystemNoticesTx(ctx context.Context, tx pgx.Tx) ([]service.SystemNotice, error) {
	rows, err := tx.Query(ctx, `
		select id, title, body, severity, enabled, extract(epoch from updated_at)::bigint
		from system_notices
		order by id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.SystemNotice
	for rows.Next() {
		var item service.SystemNotice
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Severity, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func listRiskRulesTx(ctx context.Context, tx pgx.Tx) ([]service.RiskRule, error) {
	rows, err := tx.Query(ctx, `
		select key, name, description, enabled, extract(epoch from updated_at)::bigint
		from risk_rules
		order by key asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []service.RiskRule
	for rows.Next() {
		var item service.RiskRule
		if err := rows.Scan(&item.Key, &item.Name, &item.Description, &item.Enabled, &item.UpdatedUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanOrders(rows pgx.Rows) ([]service.RechargeOrder, error) {
	var items []service.RechargeOrder
	for rows.Next() {
		var item service.RechargeOrder
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.UserNo, &item.Username, &item.AmountFen, &item.RefundedFen, &item.Channel, &item.Provider, &item.Status,
			&item.PayURL, &item.ExternalID, &item.ProviderStatus, &item.PricingVersion, &item.AgreementVersions, &item.PaidUnix, &item.LastCheckedUnix, &item.CreatedUnix, &item.UpdatedUnix,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func getOrderForUpdate(ctx context.Context, tx pgx.Tx, orderID string) (service.RechargeOrder, error) {
	var order service.RechargeOrder
	err := tx.QueryRow(ctx, `
		select id, user_id, amount_fen, refunded_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''),
		       coalesce(provider_status,''), coalesce(pricing_version,''), agreement_versions, coalesce(extract(epoch from paid_at)::bigint, 0),
		       coalesce(extract(epoch from last_checked_at)::bigint, 0),
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from recharge_orders where id = $1 for update
	`, orderID).Scan(
		&order.ID, &order.UserID, &order.AmountFen, &order.RefundedFen, &order.Channel, &order.Provider, &order.Status,
		&order.PayURL, &order.ExternalID, &order.ProviderStatus, &order.PricingVersion, &order.AgreementVersions, &order.PaidUnix, &order.LastCheckedUnix, &order.CreatedUnix, &order.UpdatedUnix,
	)
	return order, err
}

func getWalletForUpdate(ctx context.Context, tx pgx.Tx, userID string) (service.WalletSummary, error) {
	if _, err := tx.Exec(ctx, `
		insert into wallet_accounts (user_id, balance_fen, currency, updated_at)
		values ($1, 0, 'CNY', now())
		on conflict (user_id) do nothing
	`, userID); err != nil {
		return service.WalletSummary{}, err
	}
	var wallet service.WalletSummary
	err := tx.QueryRow(ctx, `
		select user_id, balance_fen, currency, extract(epoch from updated_at)::bigint
		from wallet_accounts where user_id = $1 for update
	`, userID).Scan(&wallet.UserID, &wallet.BalanceFen, &wallet.Currency, &wallet.UpdatedUnix)
	return wallet, err
}

func upsertWalletTx(ctx context.Context, tx pgx.Tx, wallet service.WalletSummary) error {
	_, err := tx.Exec(ctx, `
		update wallet_accounts set balance_fen = $2, currency = $3, updated_at = to_timestamp($4)
		where user_id = $1
	`, wallet.UserID, wallet.BalanceFen, wallet.Currency, wallet.UpdatedUnix)
	return err
}

type rowQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func getWalletTransactionByReference(
	ctx context.Context,
	q rowQueryer,
	referenceType, referenceID string,
) (service.WalletTransaction, bool, error) {
	var item service.WalletTransaction
	err := q.QueryRow(ctx, `
		select id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version,
		       extract(epoch from created_at)::bigint
		from wallet_transactions
		where reference_type = $1 and reference_id = $2
		limit 1
	`, referenceType, referenceID).Scan(
		&item.ID,
		&item.UserID,
		&item.Kind,
		&item.AmountFen,
		&item.Description,
		&item.ReferenceType,
		&item.ReferenceID,
		&item.PricingVersion,
		&item.CreatedUnix,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.WalletTransaction{}, false, nil
		}
		return service.WalletTransaction{}, false, err
	}
	return item, true, nil
}

func adminWalletMutationMatches(existing, current service.WalletTransaction) bool {
	return strings.TrimSpace(existing.UserID) == strings.TrimSpace(current.UserID) &&
		strings.TrimSpace(existing.Kind) == strings.TrimSpace(current.Kind) &&
		existing.AmountFen == current.AmountFen &&
		strings.TrimSpace(existing.Description) == strings.TrimSpace(current.Description) &&
		strings.TrimSpace(existing.ReferenceType) == strings.TrimSpace(current.ReferenceType) &&
		strings.TrimSpace(existing.ReferenceID) == strings.TrimSpace(current.ReferenceID)
}

func appendTransactionTx(ctx context.Context, tx pgx.Tx, item service.WalletTransaction) error {
	_, err := tx.Exec(ctx, `
		insert into wallet_transactions (
			id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
	`, item.ID, item.UserID, item.Kind, item.AmountFen, item.Description, item.ReferenceType, item.ReferenceID, item.PricingVersion, item.CreatedUnix)
	return err
}

func appendTransactionTxIdempotent(ctx context.Context, tx pgx.Tx, item service.WalletTransaction) (bool, error) {
	tag, err := tx.Exec(ctx, `
		insert into wallet_transactions (
			id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
		on conflict (reference_type, reference_id)
		where reference_type <> '' and reference_id <> ''
		do nothing
	`, item.ID, item.UserID, item.Kind, item.AmountFen, item.Description, item.ReferenceType, item.ReferenceID, item.PricingVersion, item.CreatedUnix)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func appendAuditLogTx(ctx context.Context, q execer, entry service.AdminAuditLog) error {
	_, err := q.Exec(ctx, `
		insert into admin_audit_logs (
			id, actor_user_id, actor_email, action, target_type, target_id, risk_level, detail, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
	`, entry.ID, entry.ActorUserID, entry.ActorEmail, entry.Action, entry.TargetType, entry.TargetID, entry.RiskLevel, entry.Detail, entry.CreatedUnix)
	return err
}

func upsertOrderTx(ctx context.Context, tx pgx.Tx, order service.RechargeOrder) error {
	_, err := tx.Exec(ctx, `
		update recharge_orders set
			amount_fen = $2,
			refunded_fen = $3,
			channel = $4,
			provider = $5,
			status = $6,
			pay_url = $7,
			external_order_id = $8,
			provider_status = $9,
			pricing_version = $10,
			agreement_versions = $11,
			paid_at = case when $12 > 0 then to_timestamp($12) else null end,
			last_checked_at = case when $13 > 0 then to_timestamp($13) else null end,
			updated_at = to_timestamp($14)
		where id = $1
	`, order.ID, order.AmountFen, order.RefundedFen, order.Channel, order.Provider, order.Status, order.PayURL, order.ExternalID, order.ProviderStatus, order.PricingVersion, order.AgreementVersions, order.PaidUnix, order.LastCheckedUnix, order.UpdatedUnix)
	return err
}
