package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/pquerna/otp/totp"
)

// Test_AdminResetTOTPAndLockout verifies the superadmin recovery hammer:
// after enrollment + lockout, the reset clears both states.
func Test_AdminResetTOTPAndLockout(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()

	u, _ := svc.CreateAdmin(ctx, "victim@x.com", "goodPass1234", "operator")

	enr, _ := svc.StartTOTPEnrollment(ctx, u.ID)
	code, _ := totp.GenerateCode(enr.Secret, time.Now())
	_ = svc.EnableTOTP(ctx, u.ID, code)

	// Trigger lockout by 5 wrong logins.
	for i := 0; i < 5; i++ {
		_, _ = svc.Authenticate(ctx, "victim@x.com", "wrongPass1234", "")
	}

	// Reset: clears TOTP + lockout + counter.
	if err := svc.AdminResetTOTPAndLockout(ctx, u.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// Now password-only login should work (TOTP gone, lockout cleared).
	got, err := svc.Authenticate(ctx, "victim@x.com", "goodPass1234", "")
	if err != nil {
		t.Fatalf("post-reset login: %v", err)
	}
	if got.TOTPEnabled {
		t.Errorf("totp_enabled should be false after reset")
	}
}

// Test_AdminResetTOTPAndLockout_NotFound verifies that resetting a
// non-existent admin returns ErrAdminNotFound (so the HTTP handler
// can map it to 404).
func Test_AdminResetTOTPAndLockout_NotFound(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	if err := svc.AdminResetTOTPAndLockout(context.Background(), 99999); err != admin.ErrAdminNotFound {
		t.Errorf("err = %v, want ErrAdminNotFound", err)
	}
}

// TestAdmin_TOTP_FullCycle exercises enroll → enable → login-with-code →
// disable.
func TestAdmin_TOTP_FullCycle(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()

	u, err := svc.CreateAdmin(ctx, "totp@x.com", "goodPass1234", "operator")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Enroll
	enr, err := svc.StartTOTPEnrollment(ctx, u.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if enr.Secret == "" || enr.URI == "" {
		t.Fatalf("empty enrollment: %+v", enr)
	}

	// Generate the current code from the secret to prove enable works.
	code, err := totp.GenerateCode(enr.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if err := svc.EnableTOTP(ctx, u.ID, code); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// With TOTP enabled, password-only login should NeedTOTP.
	if _, err := svc.Authenticate(ctx, "totp@x.com", "goodPass1234", ""); err != admin.ErrTOTPRequired {
		t.Errorf("password-only err = %v, want ErrTOTPRequired", err)
	}

	// Wrong code should look like a regular invalid login.
	if _, err := svc.Authenticate(ctx, "totp@x.com", "goodPass1234", "000000"); err != admin.ErrInvalidCredentials {
		t.Errorf("wrong code err = %v, want ErrInvalidCredentials", err)
	}

	// Right code logs in.
	good, _ := totp.GenerateCode(enr.Secret, time.Now())
	if _, err := svc.Authenticate(ctx, "totp@x.com", "goodPass1234", good); err != nil {
		t.Fatalf("login with code: %v", err)
	}

	// Disable: bad code rejected, good code accepted.
	if err := svc.DisableTOTP(ctx, u.ID, "000000"); err != admin.ErrTOTPInvalid {
		t.Errorf("disable bad code err = %v, want ErrTOTPInvalid", err)
	}
	good2, _ := totp.GenerateCode(enr.Secret, time.Now())
	if err := svc.DisableTOTP(ctx, u.ID, good2); err != nil {
		t.Fatalf("disable: %v", err)
	}

	// After disable, password-only login should work again.
	if _, err := svc.Authenticate(ctx, "totp@x.com", "goodPass1234", ""); err != nil {
		t.Errorf("post-disable login: %v", err)
	}
}
