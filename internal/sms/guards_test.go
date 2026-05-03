package sms_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
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
