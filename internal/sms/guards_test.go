package sms_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Test_DailyQuota_Enforced creates a tenant with daily_sms_limit=2,
// enqueues two OK messages, then expects the third to fail with
// ErrDailyQuotaExceeded.
func Test_DailyQuota_Enforced(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	limit := int32(2)
	tt, err := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{
		Name: "Quota", DailySMSLimit: &limit,
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	svc := sms.NewService(pool)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := svc.Enqueue(ctx, sms.EnqueueInput{
			TenantID: tt.ID, Sender: "S",
			Recipient: "56999900" + strconv.Itoa(i),
			Text:      "ok",
		}); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}

	_, err = svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S",
		Recipient: "56999900XX", Text: "over",
	})
	if err != sms.ErrDailyQuotaExceeded {
		t.Errorf("err = %v, want ErrDailyQuotaExceeded", err)
	}
}

// Test_DailyQuota_NoLimitMeansNoLimit verifies that a tenant without
// daily_sms_limit set can enqueue freely (no NULL = unlimited).
func Test_DailyQuota_NoLimitMeansNoLimit(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "NoLimit"})
	svc := sms.NewService(pool)
	for i := 0; i < 5; i++ {
		if _, err := svc.Enqueue(context.Background(), sms.EnqueueInput{
			TenantID: tt.ID, Sender: "S",
			Recipient: "56988800" + strconv.Itoa(i), Text: "x",
		}); err != nil {
			t.Errorf("attempt %d: %v", i, err)
		}
	}
}

// Test_SenderAllowList_Enforced sets allowed_senders=["Segtelco"] and
// verifies that another sender is rejected while "Segtelco" goes through.
func Test_SenderAllowList_Enforced(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "AllowList"})
	if err := ts.SetAllowedSenders(context.Background(), tt.ID, []string{"Segtelco"}); err != nil {
		t.Fatalf("set allowed_senders: %v", err)
	}
	svc := sms.NewService(pool)
	ctx := context.Background()

	// Allowed sender → OK.
	if _, err := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "Segtelco", Recipient: "56977700001", Text: "ok",
	}); err != nil {
		t.Fatalf("allowed sender: %v", err)
	}

	// Different sender → rejected.
	_, err := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "BancoChile", Recipient: "56977700002", Text: "phish",
	})
	if err != sms.ErrSenderNotAllowed {
		t.Errorf("err = %v, want ErrSenderNotAllowed", err)
	}
}

// Test_SenderAllowList_EmptyMeansAny verifies the backwards-compatible
// behaviour: if the list is empty (default), any sender is accepted.
func Test_SenderAllowList_EmptyMeansAny(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "Free"})
	svc := sms.NewService(pool)
	if _, err := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID: tt.ID, Sender: "Whatever", Recipient: "56966600001", Text: "ok",
	}); err != nil {
		t.Errorf("empty allow-list should accept any sender: %v", err)
	}
}

// Test_QuotaWarning_AuditOnce80 ensures crossing the 80% threshold
// produces exactly one tenant.quota_warning audit row, even if more
// sends happen after.
func Test_QuotaWarning_AuditOnce80(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	limit := int32(10)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{
		Name: "QuotaWarn", DailySMSLimit: &limit,
	})
	svc := sms.NewService(pool)
	ctx := context.Background()

	// 7 sends → still under 80% (threshold = ceil(0.8*10) = 8).
	for i := 0; i < 7; i++ {
		_, _ = svc.Enqueue(ctx, sms.EnqueueInput{
			TenantID: tt.ID, Sender: "S",
			Recipient: "5697700000" + strconv.Itoa(i), Text: "x",
		})
	}
	count := countQuotaWarnings(t, pool, tt.ID)
	if count != 0 {
		t.Errorf("after 7 sends, audit count = %d, want 0", count)
	}

	// 8th send crosses threshold → should log once.
	_, _ = svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "56977000017", Text: "x",
	})
	count = countQuotaWarnings(t, pool, tt.ID)
	if count != 1 {
		t.Errorf("after 8th send, audit count = %d, want 1", count)
	}

	// 9th send also above threshold → must NOT add a second row.
	_, _ = svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "56977000018", Text: "x",
	})
	count = countQuotaWarnings(t, pool, tt.ID)
	if count != 1 {
		t.Errorf("after 9th send, audit count = %d, want 1 (idempotent)", count)
	}
}

func countQuotaWarnings(t *testing.T, pool *pgxpool.Pool, tenantID int64) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_log WHERE action='tenant.quota_warning' AND target_id=$1`,
		strconv.FormatInt(tenantID, 10),
	).Scan(&n)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// Test_SetAllowedSenders_TrimsAndDedups verifies the SetAllowedSenders
// service method cleans whitespace and drops duplicates/empties.
func Test_SetAllowedSenders_TrimsAndDedups(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "Clean"})

	if err := ts.SetAllowedSenders(context.Background(), tt.ID,
		[]string{"  Acme ", "", "Acme", " Beta", "Beta", "  "},
	); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := ts.GetTenant(context.Background(), tt.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.AllowedSenders) != 2 {
		t.Errorf("len = %d, want 2 (Acme + Beta) — got %v", len(got.AllowedSenders), got.AllowedSenders)
	}
	hasAcme, hasBeta := false, false
	for _, s := range got.AllowedSenders {
		if s == "Acme" {
			hasAcme = true
		}
		if s == "Beta" {
			hasBeta = true
		}
	}
	if !hasAcme || !hasBeta {
		t.Errorf("missing Acme=%v / Beta=%v in %v", hasAcme, hasBeta, got.AllowedSenders)
	}
}
