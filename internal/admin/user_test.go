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

// TestAdmin_AuthenticateLockout exercises the 5-strikes lockout path.
// The lock check short-circuits before bcrypt, so even the correct
// password returns ErrInvalidCredentials while locked.
func TestAdmin_AuthenticateLockout(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()
	_, err := svc.CreateAdmin(ctx, "lock@x.com", "goodPass1234", "operator")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Five wrong attempts should land us in locked state.
	for i := 0; i < 5; i++ {
		if _, err := svc.Authenticate(ctx, "lock@x.com", "wrongPass1234", ""); err != admin.ErrInvalidCredentials {
			t.Fatalf("attempt %d: err = %v, want ErrInvalidCredentials", i+1, err)
		}
	}
	// The right password should now ALSO fail because the account is locked.
	if _, err := svc.Authenticate(ctx, "lock@x.com", "goodPass1234", ""); err != admin.ErrInvalidCredentials {
		t.Errorf("locked-with-right-pw err = %v, want ErrInvalidCredentials (locked short-circuit)", err)
	}
}

// TestAdmin_AuthenticateUnknownEmail verifies that an unknown email
// looks identical to a wrong password.
func TestAdmin_AuthenticateUnknownEmail(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	if _, err := svc.Authenticate(context.Background(), "ghost@x.com", "whatever1234", ""); err != admin.ErrInvalidCredentials {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

// TestAdmin_AuthenticateResetsCounter verifies that a successful login
// clears any prior fail count so the next bad password starts at 1, not 5.
func TestAdmin_AuthenticateResetsCounter(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()
	_, _ = svc.CreateAdmin(ctx, "reset@x.com", "goodPass1234", "operator")

	// 3 fails (not enough to lock yet)
	for i := 0; i < 3; i++ {
		_, _ = svc.Authenticate(ctx, "reset@x.com", "wrongPass1234", "")
	}
	// One success → counter cleared.
	if _, err := svc.Authenticate(ctx, "reset@x.com", "goodPass1234", ""); err != nil {
		t.Fatalf("good login: %v", err)
	}
	// Now 4 more wrong attempts should NOT lock (would be 4, threshold 5).
	for i := 0; i < 4; i++ {
		_, _ = svc.Authenticate(ctx, "reset@x.com", "wrongPass1234", "")
	}
	if _, err := svc.Authenticate(ctx, "reset@x.com", "goodPass1234", ""); err != nil {
		t.Errorf("should still unlock after 4 fails post-reset: %v", err)
	}
}
