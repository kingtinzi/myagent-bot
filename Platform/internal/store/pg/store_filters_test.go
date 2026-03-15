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
		"o.user_id = $1",
		"lower(o.status) = $2",
		"lower(o.provider) = $3",
		"order by o.created_at desc, o.id desc",
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

func TestBuildListOrdersQueryIncludesUserNumbers(t *testing.T) {
	query, _ := buildListOrdersQuery(service.RechargeOrderFilter{})
	for _, fragment := range []string{
		"coalesce(p.user_no, 0)",
		"left join user_profiles p on p.user_id = o.user_id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
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
		"coalesce(p.user_no, 0)",
		"where u.user_id = $1",
		"order by updated_unix desc, coalesce(nullif(p.user_no, 0), 9223372036854775807) asc, u.user_id asc",
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
		"w.user_id = $1",
		"lower(w.kind) = $2",
		"lower(coalesce(w.reference_type,'')) = $3",
		"order by w.created_at desc, w.id desc",
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

func TestBuildListWalletAdjustmentsQueryIncludesUserNumbers(t *testing.T) {
	query, _ := buildListWalletAdjustmentsQuery(service.WalletAdjustmentFilter{})
	for _, fragment := range []string{
		"coalesce(p.user_no, 0)",
		"left join user_profiles p on p.user_id = w.user_id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
}

func TestBuildListRefundRequestsQueryAppliesRefundFilters(t *testing.T) {
	query, args := buildListRefundRequestsQuery(service.RefundRequestFilter{
		UserID:  "user-1",
		OrderID: "ord-9",
		Status:  "approved_pending_payout",
	})

	for _, fragment := range []string{
		"r.user_id = $1",
		"r.order_id = $2",
		"lower(r.status) = $3",
		"order by r.created_at desc, r.id desc",
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

func TestBuildListRefundRequestsQueryIncludesUserNumbers(t *testing.T) {
	query, _ := buildListRefundRequestsQuery(service.RefundRequestFilter{})
	for _, fragment := range []string{
		"coalesce(p.user_no, 0)",
		"left join user_profiles p on p.user_id = r.user_id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
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
		"i.user_id = $1",
		"lower(i.status) = $2",
		"i.reviewed_by = $3",
		"order by i.created_at desc, i.id desc",
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

func TestBuildListInfringementReportsQueryIncludesUserNumbers(t *testing.T) {
	query, _ := buildListInfringementReportsQuery(service.InfringementReportFilter{})
	for _, fragment := range []string{
		"coalesce(p.user_no, 0)",
		"left join user_profiles p on p.user_id = i.user_id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
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

func TestBuildAdminDashboardOrdersSummaryQueryExcludesAdminPrincipalsAndUsesRecentWindow(t *testing.T) {
	query, args := buildAdminDashboardOrdersSummaryQuery(service.AdminDashboardStoreInput{
		ExcludedAdminUserIDs: []string{"admin-1"},
		ExcludedAdminEmails:  []string{"admin@example.com"},
		SinceUnix:            1710000000,
	})

	for _, fragment := range []string{
		"from recharge_orders r",
		"left join user_profiles p on p.user_id = r.user_id",
		"cardinality($1::text[]) = 0 or r.user_id <> all($1)",
		"cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2)",
		"extract(epoch from r.created_at)::bigint, 0) >= $3",
		"extract(epoch from r.paid_at)::bigint, 0) > 0",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{[]string{"admin-1"}, []string{"admin@example.com"}, int64(1710000000)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildListUsersQuerySupportsKeywordUserNumberSearch(t *testing.T) {
	query, args := buildListUsersQuery(service.UserSummaryFilter{
		Keyword: "123",
		Limit:   20,
	})

	for _, fragment := range []string{
		"coalesce(p.user_no, 0)",
		"coalesce(nullif(p.username, ''), '')",
		"p.user_no = $1",
		"lower(coalesce(p.username,'')) = $2",
		"lower(coalesce(p.email,'')) = $3",
		"u.user_id = $4",
		"order by updated_unix desc, coalesce(nullif(p.user_no, 0), 9223372036854775807) asc, u.user_id asc",
		"limit $5",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{int64(123), "123", "123", "123", 20}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestBuildAdminDashboardTopModelsQueryExcludesAdminPrincipalsAndUsesUsageAggregation(t *testing.T) {
	query, args := buildAdminDashboardTopModelsQuery(service.AdminDashboardStoreInput{
		ExcludedAdminUserIDs: []string{"admin-1"},
		ExcludedAdminEmails:  []string{"admin@example.com"},
		SinceUnix:            1710000000,
	})

	for _, fragment := range []string{
		"from chat_usage_records u",
		"left join user_profiles p on p.user_id = u.user_id",
		"created_at >= to_timestamp($3)",
		"cardinality($1::text[]) = 0 or u.user_id <> all($1)",
		"cardinality($2::text[]) = 0 or lower(coalesce(p.email, '')) <> all($2)",
		"group by u.model_id",
		"order by charged_fen desc, usage_count desc, model_id asc",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query = %q, want fragment %q", query, fragment)
		}
	}
	wantArgs := []any{[]string{"admin-1"}, []string{"admin@example.com"}, int64(1710000000)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}
