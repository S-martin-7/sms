package tenancy_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

const pepper = "test-pepper-lifecycle"

func TestAPIKey_Lifecycle(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()

	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "Acme"})

	issued, err := svc.IssueAPIKey(ctx, tt.ID, "laptop", pepper)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if issued.Token == "" || issued.Record.ID == 0 {
		t.Fatalf("bad issued: %+v", issued)
	}

	tenantID, err := svc.VerifyAPIKey(ctx, issued.Token, pepper)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tenantID != tt.ID {
		t.Errorf("tenantID = %d, want %d", tenantID, tt.ID)
	}

	if err := svc.RevokeAPIKey(ctx, issued.Record.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.VerifyAPIKey(ctx, issued.Token, pepper); err != tenancy.ErrAPIKeyRevoked {
		t.Errorf("after revoke err = %v, want ErrAPIKeyRevoked", err)
	}
}

func TestAPIKey_VerifyUnknownPrefix(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	// 51-char token (sk_live_ + 43 zeros) with the correct shape but unknown prefix
	_, err := svc.VerifyAPIKey(context.Background(), "sk_live_"+"0000000000000000000000000000000000000000000", pepper)
	if err != tenancy.ErrAPIKeyNotFound {
		t.Errorf("err = %v, want ErrAPIKeyNotFound", err)
	}
}

func TestAPIKey_VerifyMalformed(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	_, err := svc.VerifyAPIKey(context.Background(), "not-a-key", pepper)
	if err != tenancy.ErrAPIKeyInvalid {
		t.Errorf("err = %v, want ErrAPIKeyInvalid", err)
	}
}

// TestAPIKey_VerifySuspendedTenant verifies that even a valid, unrevoked
// key fails when its tenant has been suspended — the join in
// GetAPIKeyWithTenantStatus is what enforces this.
func TestAPIKey_VerifySuspendedTenant(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()

	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "ToSuspend"})
	issued, _ := svc.IssueAPIKey(ctx, tt.ID, "k", pepper)

	// Sanity: works when active.
	if _, err := svc.VerifyAPIKey(ctx, issued.Token, pepper); err != nil {
		t.Fatalf("active verify: %v", err)
	}

	// Suspend → key must now bounce.
	if err := svc.SetStatus(ctx, tt.ID, "suspended"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := svc.VerifyAPIKey(ctx, issued.Token, pepper); err != tenancy.ErrTenantSuspended {
		t.Errorf("suspended verify err = %v, want ErrTenantSuspended", err)
	}

	// Reactivating restores access.
	_ = svc.SetStatus(ctx, tt.ID, "active")
	if _, err := svc.VerifyAPIKey(ctx, issued.Token, pepper); err != nil {
		t.Errorf("reactivated verify: %v", err)
	}
}
