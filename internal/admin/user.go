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
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int64
	Email     string
	Role      string
	CreatedAt time.Time
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
	return &User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

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
	return &User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Service) CountAdmins(ctx context.Context) (int64, error) {
	return s.q.CountAdminUsers(ctx)
}
