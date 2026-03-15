package pg

import (
	"reflect"
	"strings"
	"testing"

	"openclaw/platform/internal/service"
)

func TestBuildListOrdersQueryAppliesFilterClausesAndWindow(t *testing.T) {
	query, args := buildListOrdersQuery(service.RechargeOrderFilter{
		UserID:   "user-1",
		Status:   "paid",
		Provider: "manual",
		Limit:    10,
		Offset:   20,
	})

	for _, fragment := range []string{
		"user_id = $1",
		"lower(status) = $2",
		"lower(provider) = $3",
		"order by created_at desc, id desc",
		"limit $4",
		"offset $5",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-1", "paid", "manual", 10, 20}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListUsersQueryAppliesUserWindow(t *testing.T) {
	query, args := buildListUsersQuery(service.UserSummaryFilter{
		UserID: "user-2",
		Limit:  5,
		Offset: 10,
	})

	for _, fragment := range []string{
		"select user_id from user_profiles",
		"select user_id from wallet_accounts",
		"where u.user_id = $1",
		"order by updated_unix desc, u.user_id asc",
		"limit $2",
		"offset $3",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-2", 5, 10}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListWalletAdjustmentsQueryAppliesTransactionFilters(t *testing.T) {
	query, args := buildListWalletAdjustmentsQuery(service.WalletAdjustmentFilter{
		UserID:        "user-3",
		Kind:          "debit",
		ReferenceType: "refund_request",
		Limit:         1,
	})

	for _, fragment := range []string{
		"user_id = $1",
		"lower(kind) = $2",
		"lower(coalesce(reference_type,'')) = $3",
		"order by created_at desc, id desc",
		"limit $4",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-3", "debit", "refund_request", 1}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListRefundRequestsQueryAppliesRefundFilters(t *testing.T) {
	query, args := buildListRefundRequestsQuery(service.RefundRequestFilter{
		UserID:  "user-1",
		OrderID: "ord-9",
		Status:  "approved_pending_payout",
	})

	for _, fragment := range []string{
		"user_id = $1",
		"order_id = $2",
		"lower(status) = $3",
		"order by created_at desc, id desc",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-1", "ord-9", "approved_pending_payout"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListInfringementReportsQueryAppliesReportFilters(t *testing.T) {
	query, args := buildListInfringementReportsQuery(service.InfringementReportFilter{
		UserID:     "user-2",
		Status:     "resolved",
		ReviewedBy: "admin-1",
		Limit:      50,
	})

	for _, fragment := range []string{
		"user_id = $1",
		"lower(status) = $2",
		"reviewed_by = $3",
		"order by created_at desc, id desc",
		"limit $4",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-2", "resolved", "admin-1", 50}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListChatUsageRecordsQueryAppliesUsageFiltersAndWindow(t *testing.T) {
	query, args := buildListChatUsageRecordsQuery(service.ChatUsageRecordFilter{
		UserID:    "user-2",
		ModelID:   "official-pro",
		SinceUnix: 1710000000,
		Limit:     20,
		Offset:    40,
	})

	for _, fragment := range []string{
		"user_id = $1",
		"model_id = $2",
		"created_at >= to_timestamp($3)",
		"order by created_at desc, id desc",
		"limit $4",
		"offset $5",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"user-2", "official-pro", int64(1710000000), 20, 40}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListAuditLogsQueryAppliesRichFiltersAndWindow(t *testing.T) {
	query, args := buildListAuditLogsQuery(service.AuditLogFilter{
		Action:      "admin.manual_recharge.created",
		TargetType:  "wallet_account",
		TargetID:    "user-2",
		ActorUserID: "admin-1",
		RiskLevel:   "high",
		SinceUnix:   1710000000,
		UntilUnix:   1710003600,
		Limit:       20,
		Offset:      40,
	})

	for _, fragment := range []string{
		"action = $1",
		"target_type = $2",
		"target_id = $3",
		"actor_user_id = $4",
		"risk_level = $5",
		"created_at >= to_timestamp($6)",
		"created_at <= to_timestamp($7)",
		"order by created_at desc, id desc",
		"limit $8",
		"offset $9",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{"admin.manual_recharge.created", "wallet_account", "user-2", "admin-1", "high", int64(1710000000), int64(1710003600), 20, 40}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildAdminDashboardUsersSummaryQueryExcludesAdminPrincipals(t *testing.T) {
	query, args := buildAdminDashboardUsersSummaryQuery(service.AdminDashboardStoreInput{
		ExcludedAdminUserIDs: []string{"admin-1", "admin-2"},
		ExcludedAdminEmails:  []string{"admin@example.com"},
		SinceUnix:            1710000000,
	})

	for _, fragment := range []string{
		"with user_registry as",
		"cardinality($1::text[]) = 0 or u.user_id <> all($1)",
		"cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2)",
		"extract(epoch from p.created_at)::bigint, 0) >= $3",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{[]string{"admin-1", "admin-2"}, []string{"admin@example.com"}, int64(1710000000)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildAdminDashboardOrdersSummaryQueryUsesRecentWindow(t *testing.T) {
	query, args := buildAdminDashboardOrdersSummaryQuery(service.AdminDashboardStoreInput{SinceUnix: 1710000000})

	for _, fragment := range []string{
		"from recharge_orders",
		"lower(coalesce(status, '')) in ('paid', 'refunded')",
		"extract(epoch from created_at)::bigint, 0) >= $1",
		"extract(epoch from paid_at)::bigint, 0) > 0",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{int64(1710000000)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildAdminDashboardTopModelsQueryUsesUsageAggregation(t *testing.T) {
	query, args := buildAdminDashboardTopModelsQuery(service.AdminDashboardStoreInput{SinceUnix: 1710000000})

	for _, fragment := range []string{
		"from chat_usage_records",
		"created_at >= to_timestamp($1)",
		"group by model_id",
		"order by charged_fen desc, usage_count desc, model_id asc",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{int64(1710000000)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}
