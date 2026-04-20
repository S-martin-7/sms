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
	_, err := svc.VerifyAPIKey(context.Background(), "sk_live_"+"unknown0000000000000000000000000000000000", pepper)
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
