package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/horisen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Message is the domain type surfaced by the service layer (and serialised
// to JSON in HTTP handlers).
type Message struct {
	ID           uuid.UUID
	TenantID     int64
	Sender       string
	Recipient    string
	Text         string
	DCS          horisen.DCS
	NumParts     int
	Status       string
	HorisenMsgID *string
	ErrorCode    *string
	ErrorMessage *string
	ClientRef    *string
	Attempts     int
	CreatedAt    time.Time
	SentAt       *time.Time
	FinalAt      *time.Time
}

// EnqueueInput is the payload that creates a new outbound message.
type EnqueueInput struct {
	TenantID  int64
	Sender    string
	Recipient string
	Text      string
	ClientRef string // optional
}

// Service groups message operations backed by Postgres.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

// Enqueue validates the input, detects the DCS, counts parts and inserts
// a `queued` row. The outbox worker will pick it up and call Horisen.
func (s *Service) Enqueue(ctx context.Context, in EnqueueInput) (*Message, error) {
	if in.TenantID == 0 {
		return nil, fmt.Errorf("tenant_id required")
	}
	in.Sender = strings.TrimSpace(in.Sender)
	in.Recipient = strings.TrimSpace(in.Recipient)
	if in.Sender == "" || in.Recipient == "" || in.Text == "" {
		return nil, fmt.Errorf("sender, to and text are required")
	}
	dcs := horisen.DetectDCS(in.Text)
	parts := horisen.NumParts(in.Text, dcs)

	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}

	var ref *string
	if in.ClientRef != "" {
		v := in.ClientRef
		ref = &v
	}

	row, err := s.q.CreateMessage(ctx, sqlcgen.CreateMessageParams{
		ID:        pgID,
		TenantID:  in.TenantID,
		Sender:    in.Sender,
		Recipient: in.Recipient,
		Text:      in.Text,
		Dcs:       string(dcs),
		NumParts:  int16(parts),
		ClientRef: ref,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateClientRef
		}
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return fromRow(row), nil
}

// GetForTenant returns the message scoped to the calling tenant.
// ErrNotFound if id doesn't exist OR belongs to another tenant.
func (s *Service) GetForTenant(ctx context.Context, id uuid.UUID, tenantID int64) (*Message, error) {
	row, err := s.q.GetMessageForTenant(ctx, sqlcgen.GetMessageForTenantParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		TenantID: tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	return fromRow(row), nil
}

func fromRow(r sqlcgen.Message) *Message {
	m := &Message{
		ID:           uuid.UUID(r.ID.Bytes),
		TenantID:     r.TenantID,
		Sender:       r.Sender,
		Recipient:    r.Recipient,
		Text:         r.Text,
		DCS:          horisen.DCS(r.Dcs),
		NumParts:     int(r.NumParts),
		Status:       r.Status,
		HorisenMsgID: r.HorisenMsgID,
		ErrorCode:    r.ErrorCode,
		ErrorMessage: r.ErrorMessage,
		ClientRef:    r.ClientRef,
		Attempts:     int(r.Attempts),
		CreatedAt:    r.CreatedAt.Time,
	}
	if r.SentAt.Valid {
		t := r.SentAt.Time
		m.SentAt = &t
	}
	if r.FinalAt.Valid {
		t := r.FinalAt.Time
		m.FinalAt = &t
	}
	return m
}
