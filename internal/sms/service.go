package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

// BulkResult is one row in the response of EnqueueBulk: either Msg is set
// (queued successfully) or Err is set (validation or insertion failure for
// that specific row). The two are mutually exclusive.
//
// Index in the returned slice matches the index of the input slice so the
// caller can correlate outputs back to inputs.
type BulkResult struct {
	Msg *Message
	Err error
}

// Service groups message operations backed by Postgres.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

// TenantPolicy is the snapshot of per-tenant send-time state surfaced to
// HTTP handlers (so they can emit X-Daily-Quota-* response headers). All
// fields are post-insert when returned alongside Enqueue's result.
type TenantPolicy struct {
	DailyLimit *int32 // nil = unlimited
	SentToday  int64
}

// LoadTenantPolicy returns the quota snapshot for a tenant. Cheap (one
// indexed query). HTTP handlers use it after Enqueue to surface
// quota-usage headers in the response.
func (s *Service) LoadTenantPolicy(ctx context.Context, tenantID int64) (TenantPolicy, error) {
	p, err := s.q.GetTenantSendPolicy(ctx, sqlcgen.GetTenantSendPolicyParams{
		TenantID: tenantID,
		Since:    pgtype.Timestamptz{Time: startOfDayCLT(time.Now()), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TenantPolicy{}, ErrTenantNotFound
	}
	if err != nil {
		return TenantPolicy{}, err
	}
	return TenantPolicy{DailyLimit: p.DailySmsLimit, SentToday: p.SentToday}, nil
}

// Enqueue validates the input, detects the DCS, counts parts and inserts
// a `queued` row. The outbox worker will pick it up and call Horisen.
//
// Pre-insert guards:
//   - daily_sms_limit (per-tenant): rejects with ErrDailyQuotaExceeded if
//     today's count already meets the limit. Today is defined by the
//     America/Santiago timezone — invoices and SMS rates are billed in CLP.
//   - allowed_senders (per-tenant): if the array is non-empty, the sender
//     must be one of the entries verbatim. Empty array = no restriction.
func (s *Service) Enqueue(ctx context.Context, in EnqueueInput) (*Message, error) {
	if in.TenantID == 0 {
		return nil, fmt.Errorf("tenant_id required")
	}
	in.Sender = strings.TrimSpace(in.Sender)
	in.Recipient = strings.TrimSpace(in.Recipient)
	if in.Sender == "" || in.Recipient == "" || in.Text == "" {
		return nil, fmt.Errorf("sender, to and text are required")
	}

	// One round trip: tenant policy + today's count.
	policy, err := s.q.GetTenantSendPolicy(ctx, sqlcgen.GetTenantSendPolicyParams{
		TenantID: in.TenantID,
		Since:    pgtype.Timestamptz{Time: startOfDayCLT(time.Now()), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load tenant policy: %w", err)
	}
	if policy.DailySmsLimit != nil && policy.SentToday >= int64(*policy.DailySmsLimit) {
		return nil, ErrDailyQuotaExceeded
	}
	if len(policy.AllowedSenders) > 0 && !containsString(policy.AllowedSenders, in.Sender) {
		return nil, ErrSenderNotAllowed
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

	// Quota-warning audit: if this insert crossed the 80% threshold for
	// the first time today, log it once. Cheap when not crossing (just
	// arithmetic); when crossing, one indexed audit_log lookup.
	if policy.DailySmsLimit != nil {
		s.maybeLogQuotaWarning(ctx, in.TenantID, policy.SentToday, int64(*policy.DailySmsLimit))
	}

	return fromRow(row), nil
}

// maybeLogQuotaWarning fires a one-shot audit log entry the moment a
// tenant's daily usage crosses 80%. Idempotent per tenant per day —
// further sends within the same day don't re-log. Best-effort: errors
// are swallowed (this is observability, not control flow).
func (s *Service) maybeLogQuotaWarning(ctx context.Context, tenantID, before, limit int64) {
	if limit <= 0 {
		return
	}
	threshold := (limit*80 + 99) / 100 // ceil(0.8*limit) without floats
	after := before + 1
	if before >= threshold || after < threshold {
		return // didn't cross with this insert
	}
	since := pgtype.Timestamptz{Time: startOfDayCLT(time.Now()), Valid: true}
	target := strconv.FormatInt(tenantID, 10)
	exists, err := s.q.HasAuditLogSinceForTarget(ctx, sqlcgen.HasAuditLogSinceForTargetParams{
		Action:   "tenant.quota_warning",
		TargetID: target,
		Since:    since,
	})
	if err != nil || exists {
		return
	}
	meta, _ := json.Marshal(map[string]any{
		"sent_today": after,
		"limit":      limit,
		"threshold":  threshold,
	})
	tt := "tenant"
	_ = s.q.AppendAuditLog(ctx, sqlcgen.AppendAuditLogParams{
		Action:     "tenant.quota_warning",
		TargetType: &tt,
		TargetID:   &target,
		Metadata:   meta,
	})
}

// EnqueueBulk inserts N messages, returning per-row results. Rows that fail
// validation or hit ErrDuplicateClientRef are reported in the result with
// `Err` set; successful rows have `Msg` set. Length and order of the
// returned slice match the input.
//
// Partial-accept semantics: a single bad row never blocks the others. We
// don't wrap the inserts in a single transaction because a failed INSERT
// would poison the whole tx; instead each CreateMessage is its own atomic
// op (no inter-row state to coordinate).
func (s *Service) EnqueueBulk(ctx context.Context, inputs []EnqueueInput) []BulkResult {
	out := make([]BulkResult, len(inputs))
	for i, in := range inputs {
		msg, err := s.Enqueue(ctx, in)
		if err != nil {
			out[i] = BulkResult{Err: err}
			continue
		}
		out[i] = BulkResult{Msg: msg}
	}
	return out
}

// ListOpts are the filters for ListMessages. Empty fields are no-ops.
type ListOpts struct {
	Status          string    // exact match (queued, sent, delivered, ...)
	Recipient       string    // exact match (E.164 without '+')
	ClientRef       string    // exact match (tenant idempotency key)
	From            time.Time // inclusive lower bound on created_at; zero = no bound
	To              time.Time // exclusive upper bound on created_at; zero = no bound
	CursorCreatedAt time.Time // zero = from newest
	CursorID        uuid.UUID // pair with CursorCreatedAt for tuple comparison
	Limit           int       // capped to MaxListLimit; 0 → DefaultListLimit
}

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// ListMessages returns messages newest-first for the tenant, applying the
// given filters. Use the last item's (CreatedAt, ID) as the next cursor
// when len(result) == Limit.
func (s *Service) ListMessages(ctx context.Context, tenantID int64, opts ListOpts) ([]*Message, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultListLimit
	}
	if opts.Limit > MaxListLimit {
		opts.Limit = MaxListLimit
	}

	params := sqlcgen.ListMessagesFilteredParams{
		TenantID: tenantID,
		Lim:      int32(opts.Limit),
	}
	if !opts.CursorCreatedAt.IsZero() {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: opts.CursorCreatedAt, Valid: true}
		params.CursorID = pgtype.UUID{Bytes: opts.CursorID, Valid: true}
	}
	if opts.Status != "" {
		v := opts.Status
		params.Status = &v
	}
	if opts.Recipient != "" {
		v := opts.Recipient
		params.Recipient = &v
	}
	if opts.ClientRef != "" {
		v := opts.ClientRef
		params.ClientRef = &v
	}
	if !opts.From.IsZero() {
		params.FromTime = pgtype.Timestamptz{Time: opts.From, Valid: true}
	}
	if !opts.To.IsZero() {
		params.ToTime = pgtype.Timestamptz{Time: opts.To, Valid: true}
	}

	rows, err := s.q.ListMessagesFiltered(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	out := make([]*Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// AdminListOpts is ListOpts plus an optional TenantID filter — admins
// can search across tenants or scope to one. Zero TenantID = all tenants.
type AdminListOpts struct {
	TenantID int64 // 0 = no tenant filter
	ListOpts
}

// AdminListMessages is the cross-tenant variant of ListMessages used by
// the /admin/messages endpoint.
func (s *Service) AdminListMessages(ctx context.Context, opts AdminListOpts) ([]*Message, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultListLimit
	}
	if opts.Limit > MaxListLimit {
		opts.Limit = MaxListLimit
	}
	params := sqlcgen.ListMessagesAdminFilteredParams{Lim: int32(opts.Limit)}
	if opts.TenantID != 0 {
		v := opts.TenantID
		params.TenantID = &v
	}
	if !opts.CursorCreatedAt.IsZero() {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: opts.CursorCreatedAt, Valid: true}
		params.CursorID = pgtype.UUID{Bytes: opts.CursorID, Valid: true}
	}
	if opts.Status != "" {
		v := opts.Status
		params.Status = &v
	}
	if opts.Recipient != "" {
		v := opts.Recipient
		params.Recipient = &v
	}
	if opts.ClientRef != "" {
		v := opts.ClientRef
		params.ClientRef = &v
	}
	if !opts.From.IsZero() {
		params.FromTime = pgtype.Timestamptz{Time: opts.From, Valid: true}
	}
	if !opts.To.IsZero() {
		params.ToTime = pgtype.Timestamptz{Time: opts.To, Valid: true}
	}
	rows, err := s.q.ListMessagesAdminFiltered(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("admin list messages: %w", err)
	}
	out := make([]*Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
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

// startOfDayCLT is midnight in America/Santiago for the day containing t.
// Falls back to UTC if the tzdata is missing (shouldn't happen on Ubuntu
// with tzdata installed). The daily quota window is intentionally a
// civil-day boundary, not a rolling 24h window — easier to reason about
// for billing.
func startOfDayCLT(t time.Time) time.Time {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		loc = time.UTC
	}
	tt := t.In(loc)
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, loc)
}

// SecondsUntilEndOfDayCLT returns how many seconds until the start of
// tomorrow in America/Santiago — used by the handler to set Retry-After
// when the daily quota is hit.
func SecondsUntilEndOfDayCLT(now time.Time) int {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		loc = time.UTC
	}
	nn := now.In(loc)
	tomorrow := time.Date(nn.Year(), nn.Month(), nn.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
	d := tomorrow.Sub(nn)
	if d < 0 {
		return 0
	}
	return int(d.Seconds())
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
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
