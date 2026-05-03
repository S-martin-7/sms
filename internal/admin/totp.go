package admin

import (
	"context"
	"errors"
	"fmt"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
)

// TOTPEnrollment is what the dashboard needs to render the QR. The
// secret is also returned so users with a manual-entry authenticator
// can copy it.
type TOTPEnrollment struct {
	Secret string // base32, what the user types into the app
	URI    string // otpauth://totp/...  — encode this as a QR code
}

// totpIssuer is what shows up in the authenticator app's account name.
const totpIssuer = "SMS Gateway"

// StartTOTPEnrollment generates a fresh secret for the admin and stores
// it WITHOUT enabling TOTP. The caller must follow up with EnableTOTP
// after the user proves they scanned the QR by submitting a valid code.
//
// Re-running this on an enrolled account is allowed and overwrites any
// pending (un-enabled) secret — but if totp_enabled is already true we
// refuse, to avoid silently replacing a working second factor.
func (s *Service) StartTOTPEnrollment(ctx context.Context, adminID int64) (*TOTPEnrollment, error) {
	row, err := s.getAdminByID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if row.TotpEnabled {
		return nil, errors.New("admin: totp already enabled — disable it first to re-enroll")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: row.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("totp generate: %w", err)
	}
	if err := s.q.SetAdminTOTPSecret(ctx, sqlcgen.SetAdminTOTPSecretParams{
		ID:         adminID,
		TotpSecret: ptr(key.Secret()),
	}); err != nil {
		return nil, fmt.Errorf("store totp secret: %w", err)
	}
	return &TOTPEnrollment{Secret: key.Secret(), URI: key.URL()}, nil
}

// EnableTOTP flips the totp_enabled bit, but only after verifying the
// user can produce a valid code from the secret stored by
// StartTOTPEnrollment. Returns ErrTOTPInvalid for a bad code.
func (s *Service) EnableTOTP(ctx context.Context, adminID int64, code string) error {
	row, err := s.getAdminByID(ctx, adminID)
	if err != nil {
		return err
	}
	if row.TotpSecret == nil || *row.TotpSecret == "" {
		return ErrTOTPNotEnrolled
	}
	if !totp.Validate(code, *row.TotpSecret) {
		return ErrTOTPInvalid
	}
	return s.q.SetAdminTOTPEnabled(ctx, sqlcgen.SetAdminTOTPEnabledParams{
		ID:          adminID,
		TotpEnabled: true,
	})
}

// DisableTOTP requires a valid current code (proving the user still
// controls the authenticator app) before clearing the secret. A lost
// authenticator should be reset by a superadmin via a future
// /admin/users/{id}/totp/reset endpoint — not by self-service here.
func (s *Service) DisableTOTP(ctx context.Context, adminID int64, code string) error {
	row, err := s.getAdminByID(ctx, adminID)
	if err != nil {
		return err
	}
	if !row.TotpEnabled || row.TotpSecret == nil {
		return ErrTOTPNotEnrolled
	}
	if !totp.Validate(code, *row.TotpSecret) {
		return ErrTOTPInvalid
	}
	return s.q.SetAdminTOTPEnabled(ctx, sqlcgen.SetAdminTOTPEnabledParams{
		ID:          adminID,
		TotpEnabled: false,
	})
}

// AdminResetTOTPAndLockout is the superadmin escape hatch: clears the
// target admin's TOTP enrollment and any active lockout. Used when a
// user loses their authenticator app or a lockout needs manual override.
// The caller is responsible for verifying the actor's role first.
func (s *Service) AdminResetTOTPAndLockout(ctx context.Context, targetID int64) error {
	// Confirm target exists so we surface a friendly 404 instead of a
	// silent no-op UPDATE.
	if _, err := s.getAdminByID(ctx, targetID); err != nil {
		return err
	}
	return s.q.ResetAdminTOTPAndLockout(ctx, targetID)
}

// ListAdmins returns the full admin user table sorted by id. Used by the
// superadmin recovery UI so they can pick which account to reset.
func (s *Service) ListAdmins(ctx context.Context) ([]*User, error) {
	rows, err := s.q.ListAdminUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*User, 0, len(rows))
	for _, r := range rows {
		out = append(out, userFromRow(r))
	}
	return out, nil
}

func (s *Service) getAdminByID(ctx context.Context, id int64) (sqlcgen.AdminUser, error) {
	row, err := s.q.GetAdminUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlcgen.AdminUser{}, ErrAdminNotFound
	}
	if err != nil {
		return sqlcgen.AdminUser{}, fmt.Errorf("lookup admin: %w", err)
	}
	return row, nil
}

func ptr[T any](v T) *T { return &v }
