// Package events implements the polling-API feed: persisted, tenant-scoped
// records of every event we'd otherwise only have delivered via webhook.
//
// Why a separate table from webhook_deliveries? webhook_deliveries is per
// endpoint and gets cascade-deleted with its endpoint. The events table is
// the canonical record so a tenant with no webhooks (or with a dead
// endpoint) can still backfill via GET /v1/events.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidType = errors.New("events: unknown event type")
)

// Event is the domain shape of a persisted event row.
type Event struct {
	ID        int64
	TenantID  int64
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// Service is a thin Postgres-backed events store.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

// Create marshals payload as JSON and inserts it. Type must be one of the
// known constants in webhooks.AllEvents — we share the whitelist so the
// polling feed and webhook fan-out stay in sync.
func (s *Service) Create(ctx context.Context, tenantID int64, eventType string, payload any) (*Event, error) {
	if _, ok := webhooks.AllEvents[eventType]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidType, eventType)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	row, err := s.q.CreateEvent(ctx, sqlcgen.CreateEventParams{
		TenantID: tenantID,
		Type:     eventType,
		Payload:  body,
	})
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}
	return &Event{
		ID:        row.ID,
		TenantID:  row.TenantID,
		Type:      row.Type,
		Payload:   row.Payload,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// ListOpts are the filter knobs for List.
type ListOpts struct {
	Types    []string  // empty = no type filter
	From     time.Time // zero = no lower bound
	To       time.Time // zero = no upper bound
	CursorID int64     // 0 = from newest
	Limit    int       // capped to MaxLimit; 0 → DefaultLimit
}

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// List returns events for the tenant, newest first, respecting filters.
//
// Returns at most Limit items. Use the last item's ID as the next cursor
// (encoded by the HTTP layer) when len(result) == Limit.
func (s *Service) List(ctx context.Context, tenantID int64, opts ListOpts) ([]*Event, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultLimit
	}
	if opts.Limit > MaxLimit {
		opts.Limit = MaxLimit
	}

	// Validate any explicit types up front so we 400 early instead of
	// silently returning [].
	for _, t := range opts.Types {
		if _, ok := webhooks.AllEvents[t]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidType, t)
		}
	}

	params := sqlcgen.ListEventsByTenantParams{
		TenantID: tenantID,
		CursorID: opts.CursorID,
		Lim:      int32(opts.Limit),
	}
	if len(opts.Types) > 0 {
		params.Types = opts.Types
	}
	if !opts.From.IsZero() {
		params.FromTime = pgtype.Timestamptz{Time: opts.From, Valid: true}
	}
	if !opts.To.IsZero() {
		params.ToTime = pgtype.Timestamptz{Time: opts.To, Valid: true}
	}

	rows, err := s.q.ListEventsByTenant(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	out := make([]*Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, &Event{
			ID:        r.ID,
			TenantID:  r.TenantID,
			Type:      r.Type,
			Payload:   r.Payload,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}
