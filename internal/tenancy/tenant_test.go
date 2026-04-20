package tenancy_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func TestTenant_CreateAndGet(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()

	tt, err := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tt.ID == 0 {
		t.Fatal("id not assigned")
	}
	if tt.Name != "Acme" {
		t.Errorf("name = %q, want Acme", tt.Name)
	}
	if tt.Status != "active" {
		t.Errorf("status = %q, want active", tt.Status)
	}

	got, err := svc.GetTenant(ctx, tt.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != tt.ID || got.Name != "Acme" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestTenant_GetNotFound(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	if _, err := svc.GetTenant(context.Background(), 99999); err != tenancy.ErrTenantNotFound {
		t.Errorf("err = %v, want ErrTenantNotFound", err)
	}
}

func TestTenant_SuspendActivate(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()
	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "X"})

	if err := svc.SetStatus(ctx, tt.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetTenant(ctx, tt.ID)
	if got.Status != "suspended" {
		t.Errorf("status = %q, want suspended", got.Status)
	}
}
