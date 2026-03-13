package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
			update admin_users set active = false where lower(email) <> all($1)
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
			select 1 from admin_users where active = true and (user_id = $1 or lower(email) = $2)
		)
	`, userID, email).Scan(&exists)
	return exists, err
}

func (s *Store) RecordAgreementAcceptance(ctx context.Context, acceptance service.AgreementAcceptance) error {
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
	_, err := s.pool.Exec(ctx, `
		insert into chat_usage_records (
			id, user_id, model_id, pricing_version, prompt_tokens, completion_tokens,
			charged_fen, fallback_applied, request_kind, agreement_versions, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, to_timestamp($11))
	`, usage.ID, usage.UserID, usage.ModelID, usage.PricingVersion, usage.PromptTokens, usage.CompletionTokens,
		usage.ChargedFen, usage.FallbackApplied, usage.RequestKind, usage.AgreementVersions, usage.CreatedUnix)
	return err
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
	_, err := s.pool.Exec(ctx, `
		insert into admin_audit_logs (
			id, actor_user_id, actor_email, action, target_type, target_id, risk_level, detail, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
	`, entry.ID, entry.ActorUserID, entry.ActorEmail, entry.Action, entry.TargetType, entry.TargetID, entry.RiskLevel, entry.Detail, entry.CreatedUnix)
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, filter service.AuditLogFilter) ([]service.AdminAuditLog, error) {
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
	if filter.ActorUserID != "" {
		clauses = append(clauses, "actor_user_id = "+next(filter.ActorUserID))
	}
	rows, err := s.pool.Query(ctx, `
		select id, actor_user_id, actor_email, action, target_type, target_id, risk_level, detail,
		       extract(epoch from created_at)::bigint
		from admin_audit_logs
		where `+strings.Join(clauses, " and ")+`
		order by created_at desc
	`, args...)
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

	if status == "refunded" && request.SettledUnix == 0 {
		order, err := getOrderForUpdate(ctx, tx, request.OrderID)
		if err != nil {
			return service.RefundRequest{}, err
		}
		wallet, err := getWalletForUpdate(ctx, tx, request.UserID)
		if err != nil {
			return service.RefundRequest{}, err
		}
		if wallet.BalanceFen < request.AmountFen {
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

func buildListOrdersQuery(filter service.RechargeOrderFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select id, user_id, amount_fen, refunded_fen, channel, provider, status, coalesce(pay_url,''), coalesce(external_order_id,''),
		       coalesce(provider_status,''), coalesce(pricing_version,''), agreement_versions,
		       coalesce(extract(epoch from paid_at)::bigint, 0),
		       coalesce(extract(epoch from last_checked_at)::bigint, 0),
		       extract(epoch from created_at)::bigint,
		       extract(epoch from updated_at)::bigint
		from recharge_orders
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(status) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Provider); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(provider) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListUsersQuery(filter service.UserSummaryFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select w.user_id, w.balance_fen, w.currency, extract(epoch from w.updated_at)::bigint,
		       coalesce(o.order_count, 0), coalesce(r.refund_count, 0),
		       coalesce(o.last_order_unix, 0), coalesce(r.last_refund_unix, 0)
		from wallet_accounts w
		left join (
		  select user_id, count(*)::int as order_count, max(extract(epoch from created_at)::bigint) as last_order_unix
		  from recharge_orders group by user_id
		) o on o.user_id = w.user_id
		left join (
		  select user_id, count(*)::int as refund_count, max(extract(epoch from created_at)::bigint) as last_refund_unix
		  from refund_requests group by user_id
		) r on r.user_id = w.user_id
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("w.user_id = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "w.updated_at desc, w.user_id asc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListWalletAdjustmentsQuery(filter service.WalletAdjustmentFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version,
		       extract(epoch from created_at)::bigint
		from wallet_transactions
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Kind); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(kind) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.ReferenceType); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(coalesce(reference_type,'')) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListRefundRequestsQuery(filter service.RefundRequestFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select id, user_id, order_id, amount_fen, reason, status, review_note, reviewed_by, refund_provider,
		       coalesce(external_refund_id,''), coalesce(external_status,''), coalesce(failure_reason,''),
		       coalesce(extract(epoch from settled_at)::bigint, 0),
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from refund_requests
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.OrderID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("order_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(status) = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
}

func buildListInfringementReportsQuery(filter service.InfringementReportFilter) (string, []any) {
	query := strings.Builder{}
	query.WriteString(`
		select id, user_id, subject, description, evidence_urls, status, resolution, reviewed_by,
		       extract(epoch from created_at)::bigint, extract(epoch from updated_at)::bigint
		from infringement_reports
	`)
	args := []any{}
	clauses := []string{}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		args = append(args, strings.ToLower(value))
		clauses = append(clauses, fmt.Sprintf("lower(status) = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.ReviewedBy); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("reviewed_by = $%d", len(args)))
	}
	appendListClauses(&query, &args, clauses, "created_at desc, id desc", filter.Limit, filter.Offset)
	return query.String(), args
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
		if err := rows.Scan(&item.UserID, &item.BalanceFen, &item.Currency, &item.UpdatedUnix, &item.OrderCount, &item.RefundCount, &item.LastOrderUnix, &item.LastRefundUnix); err != nil {
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
		if err := rows.Scan(&item.ID, &item.UserID, &item.Kind, &item.AmountFen, &item.Description, &item.ReferenceType, &item.ReferenceID, &item.PricingVersion, &item.CreatedUnix); err != nil {
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
		if err := rows.Scan(&item.ID, &item.UserID, &item.OrderID, &item.AmountFen, &item.Reason, &item.Status, &item.ReviewNote, &item.ReviewedBy, &item.RefundProvider, &item.ExternalRefundID, &item.ExternalStatus, &item.FailureReason, &item.SettledUnix, &item.CreatedUnix, &item.UpdatedUnix); err != nil {
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
		if err := rows.Scan(&item.ID, &item.UserID, &item.Subject, &item.Description, &item.EvidenceURLs, &item.Status, &item.Resolution, &item.ReviewedBy, &item.CreatedUnix, &item.UpdatedUnix); err != nil {
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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

func scanOrders(rows pgx.Rows) ([]service.RechargeOrder, error) {
	var items []service.RechargeOrder
	for rows.Next() {
		var item service.RechargeOrder
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.AmountFen, &item.RefundedFen, &item.Channel, &item.Provider, &item.Status,
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

func appendTransactionTx(ctx context.Context, tx pgx.Tx, item service.WalletTransaction) error {
	_, err := tx.Exec(ctx, `
		insert into wallet_transactions (
			id, user_id, kind, amount_fen, description, reference_type, reference_id, pricing_version, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, to_timestamp($9))
	`, item.ID, item.UserID, item.Kind, item.AmountFen, item.Description, item.ReferenceType, item.ReferenceID, item.PricingVersion, item.CreatedUnix)
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
