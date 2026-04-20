package admin_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/db"
)

func TestAdmin_CreateAndLogin(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4) // low bcrypt cost for tests
	ctx := context.Background()

	u, err := svc.CreateAdmin(ctx, "a@b.com", "p1234567", "superadmin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("id not set")
	}

	got, err := svc.VerifyPassword(ctx, "a@b.com", "p1234567")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("id = %d, want %d", got.ID, u.ID)
	}

	if _, err := svc.VerifyPassword(ctx, "a@b.com", "nope"); err != admin.ErrInvalidCredentials {
		t.Errorf("wrong pw err = %v, want ErrInvalidCredentials", err)
	}

	if _, err := svc.VerifyPassword(ctx, "no@x.com", "x"); err != admin.ErrInvalidCredentials {
		t.Errorf("unknown email err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAdmin_DuplicateEmail(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()
	_, _ = svc.CreateAdmin(ctx, "a@b.com", "x12345678", "superadmin")
	_, err := svc.CreateAdmin(ctx, "a@b.com", "y12345678", "operator")
	if err != admin.ErrAdminExists {
		t.Errorf("err = %v, want ErrAdminExists", err)
	}
}
