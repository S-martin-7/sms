package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID            int64
	Name          string
	Status        string
	DailySMSLimit *int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateTenantInput struct {
	Name          string
	DailySMSLimit *int32
}

// Service groups tenant and api-key operations backed by Postgres.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

func (s *Service) CreateTenant(ctx context.Context, in CreateTenantInput) (*Tenant, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	row, err := s.q.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		Name:          in.Name,
		DailySmsLimit: in.DailySMSLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return tenantFromRow(row), nil
}

func (s *Service) GetTenant(ctx context.Context, id int64) (*Tenant, error) {
	row, err := s.q.GetTenantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return tenantFromRow(row), nil
}

func (s *Service) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.q.ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	out := make([]*Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenantFromRow(r))
	}
	return out, nil
}

func (s *Service) SetStatus(ctx context.Context, id int64, status string) error {
	if status != "active" && status != "suspended" {
		return fmt.Errorf("invalid status %q", status)
	}
	return s.q.SetTenantStatus(ctx, sqlcgen.SetTenantStatusParams{ID: id, Status: status})
}

func tenantFromRow(r sqlcgen.Tenant) *Tenant {
	return &Tenant{
		ID:            r.ID,
		Name:          r.Name,
		Status:        r.Status,
		DailySMSLimit: r.DailySmsLimit,
		CreatedAt:     r.CreatedAt.Time,
		UpdatedAt:     r.UpdatedAt.Time,
	}
}
