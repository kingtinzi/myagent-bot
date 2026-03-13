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
		"where w.user_id = $1",
		"order by w.updated_at desc, w.user_id asc",
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
