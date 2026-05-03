package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// lockoutThreshold/lockoutDuration are also enforced in SQL by
// BumpAdminFailedAttempts; we duplicate them here only for the lock check
// short-circuit before bcrypt. Keep in sync with queries.sql.
const (
	lockoutThreshold = 5
	lockoutDuration  = 15 * time.Minute
)

type User struct {
	ID          int64
	Email       string
	Role        string
	TOTPEnabled bool
	CreatedAt   time.Time
}

type Service struct {
	pool       *pgxpool.Pool
	q          *sqlcgen.Queries
	bcryptCost int
}

func NewService(pool *pgxpool.Pool, bcryptCost int) *Service {
	if bcryptCost < bcrypt.MinCost {
		bcryptCost = bcrypt.DefaultCost
	}
	return &Service{pool: pool, q: sqlcgen.New(pool), bcryptCost: bcryptCost}
}

func (s *Service) CreateAdmin(ctx context.Context, email, password, role string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("email required")
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if role != "superadmin" && role != "operator" {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	row, err := s.q.CreateAdminUser(ctx, sqlcgen.CreateAdminUserParams{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAdminExists
		}
		return nil, fmt.Errorf("insert admin: %w", err)
	}
	return userFromRow(row), nil
}

// Authenticate runs the full login pipeline: lockout check → password →
// (optional) TOTP. It returns the user on full success, ErrTOTPRequired
// when the password is OK but the account has TOTP enabled and no code
// was supplied, or ErrInvalidCredentials for every other failure mode
// (bad password, locked, unknown email, bad TOTP) — never leak which
// branch failed.
func (s *Service) Authenticate(ctx context.Context, email, password, totpCode string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row, err := s.q.GetAdminUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup admin: %w", err)
	}

	// Locked? Short-circuit BEFORE bcrypt so an attacker can't time-probe.
	if row.LockedUntil.Valid && row.LockedUntil.Time.After(time.Now()) {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		_, _ = s.q.BumpAdminFailedAttempts(ctx, row.ID)
		return nil, ErrInvalidCredentials
	}

	// Password OK → if TOTP is enabled, gate on the code.
	if row.TotpEnabled {
		if totpCode == "" {
			// Tell the caller to ask for the code. Do NOT bump the
			// failure counter — a missing code on a valid password is
			// an expected protocol step, not a brute-force attempt.
			return nil, ErrTOTPRequired
		}
		secret := ""
		if row.TotpSecret != nil {
			secret = *row.TotpSecret
		}
		if !totp.Validate(totpCode, secret) {
			_, _ = s.q.BumpAdminFailedAttempts(ctx, row.ID)
			return nil, ErrInvalidCredentials
		}
	}

	if err := s.q.ResetAdminLoginState(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("reset login state: %w", err)
	}
	return userFromRow(row), nil
}

// VerifyPassword is kept for the seed/CLI path which doesn't go through
// the locking + TOTP pipeline. Tests still call it. NOT used by the
// login HTTP handler anymore.
func (s *Service) VerifyPassword(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row, err := s.q.GetAdminUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup admin: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return userFromRow(row), nil
}

func (s *Service) CountAdmins(ctx context.Context) (int64, error) {
	return s.q.CountAdminUsers(ctx)
}

// GetAdminByID is the public-facing read of a single admin row used by
// /admin/me to surface (email, role, totp_enabled) to the dashboard.
func (s *Service) GetAdminByID(ctx context.Context, id int64) (*User, error) {
	row, err := s.getAdminByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return userFromRow(row), nil
}

func userFromRow(r sqlcgen.AdminUser) *User {
	return &User{
		ID:          r.ID,
		Email:       r.Email,
		Role:        r.Role,
		TOTPEnabled: r.TotpEnabled,
		CreatedAt:   r.CreatedAt.Time,
	}
}
