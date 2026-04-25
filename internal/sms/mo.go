package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MO is the parsed shape of a Horisen MO (mobile-originated SMS) callback.
//
// The PLAN.md guess of the field names follows the same convention as the
// DLR payload (custom keys, msgId, etc.). When real MO traffic arrives we
// adjust the json tags here and add a captured-payload test, like we did
// with the DLR after seeing the live shape.
type MO struct {
	HorisenID string          `json:"id,omitempty"`         // some providers use "id"
	HorisenMsgID string       `json:"msgId,omitempty"`      // others use "msgId" — accept either
	Source    string          `json:"src"`                   // sender MSISDN
	Dest      string          `json:"dst"`                   // our number (used for tenant routing)
	Text      string          `json:"text"`
	DCS       string          `json:"dcs,omitempty"`         // GSM | UCS — informational
	ReceivedAt json.RawMessage `json:"receivedAt,omitempty"` // ISO8601 string OR unix int — accept either
	Custom    map[string]any  `json:"custom,omitempty"`
}

// horisenID returns the best identifier we have to dedupe duplicate MOs.
func (m *MO) horisenID() string {
	if m.HorisenID != "" {
		return m.HorisenID
	}
	return m.HorisenMsgID
}

// receivedAtTime parses the receivedAt field, accepting both ISO8601 strings
// and unix epoch (seconds) integers — Horisen has been seen using both.
// Falls back to "now" if the field is absent or unparseable.
func (m *MO) receivedAtTime() time.Time {
	if len(m.ReceivedAt) == 0 {
		return time.Now().UTC()
	}
	var s string
	if json.Unmarshal(m.ReceivedAt, &s) == nil && s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
	}
	var n int64
	if json.Unmarshal(m.ReceivedAt, &n) == nil && n > 0 {
		return time.Unix(n, 0).UTC()
	}
	return time.Now().UTC()
}

// ParseMO decodes a Horisen MO JSON body. Returns ErrInvalidMO if the body
// is malformed or missing src/dst/text — the bare minimum we need to route
// and persist.
func ParseMO(body []byte) (*MO, error) {
	var m MO
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMO, err)
	}
	m.Source = strings.TrimSpace(m.Source)
	m.Dest = strings.TrimSpace(m.Dest)
	if m.Source == "" || m.Dest == "" {
		return nil, fmt.Errorf("%w: src and dst required", ErrInvalidMO)
	}
	if m.Text == "" {
		return nil, fmt.Errorf("%w: text required", ErrInvalidMO)
	}
	return &m, nil
}

// InboundMessage is the persisted shape of an MO surfaced via API/webhook.
type InboundMessage struct {
	ID         uuid.UUID
	TenantID   int64
	HorisenID  string
	Src        string
	Dst        string
	Text       string
	DCS        string
	ReceivedAt time.Time
	CreatedAt  time.Time
}

// ApplyMOResult tells the caller what happened.
type ApplyMOResult struct {
	Inbound  *InboundMessage
	Skipped  bool   // true when there's no inbound_numbers route for the dst
	SkipReason string
}

// ApplyMO routes the MO to a tenant via inbound_numbers and inserts a row
// into inbound_messages. Idempotent on horisen_id (duplicate MOs return the
// existing row with Skipped=false but the same ID).
//
// Returns ApplyMOResult{Skipped:true} if the destination MSISDN is not
// mapped to any tenant — Horisen will be acked with 200 anyway so it
// doesn't keep retrying, and the MO is dropped.
func (s *Service) ApplyMO(ctx context.Context, m *MO) (*ApplyMOResult, error) {
	row, err := s.q.GetInboundNumber(ctx, m.Dest)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ApplyMOResult{
			Skipped:    true,
			SkipReason: "no inbound_numbers route for dst=" + m.Dest,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup inbound number: %w", err)
	}

	id := uuid.New()
	var hID *string
	if hid := m.horisenID(); hid != "" {
		hID = &hid
	}
	var dcs *string
	if m.DCS != "" {
		v := m.DCS
		dcs = &v
	}
	_ = dcs // currently we don't store DCS via the query — keep parsed for logging

	created, err := s.q.CreateInboundMessage(ctx, sqlcgen.CreateInboundMessageParams{
		ID:         pgtype.UUID{Bytes: id, Valid: true},
		TenantID:   row.TenantID,
		HorisenID:  hID,
		Src:        m.Source,
		Dst:        m.Dest,
		Text:       m.Text,
		Dcs:        nullableString(m.DCS),
		ReceivedAt: pgtype.Timestamptz{Time: m.receivedAtTime(), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("insert inbound message: %w", err)
	}

	return &ApplyMOResult{Inbound: inboundFromRow(created)}, nil
}

// AssignInboundNumber maps an MSISDN to a tenant (operator action — used
// by smsctl until the admin API exists).
func (s *Service) AssignInboundNumber(ctx context.Context, msisdn string, tenantID int64, label string) (*InboundNumber, error) {
	msisdn = strings.TrimSpace(msisdn)
	if msisdn == "" {
		return nil, fmt.Errorf("msisdn required")
	}
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant_id required")
	}
	row, err := s.q.CreateInboundNumber(ctx, sqlcgen.CreateInboundNumberParams{
		Msisdn:   msisdn,
		TenantID: tenantID,
		Label:    nullableString(label),
	})
	if err != nil {
		return nil, fmt.Errorf("create inbound number: %w", err)
	}
	return &InboundNumber{
		MSISDN:    row.Msisdn,
		TenantID:  row.TenantID,
		Label:     deref(row.Label),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// ListInboundNumbers returns every routing entry across all tenants.
// Used by the smsctl `inbound list` operator command.
func (s *Service) ListInboundNumbers(ctx context.Context) ([]*InboundNumber, error) {
	rows, err := s.q.ListInboundNumbersAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list inbound numbers: %w", err)
	}
	out := make([]*InboundNumber, 0, len(rows))
	for _, r := range rows {
		out = append(out, &InboundNumber{
			MSISDN:    r.Msisdn,
			TenantID:  r.TenantID,
			Label:     deref(r.Label),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// UnassignInboundNumber removes the routing for an MSISDN. Subsequent MOs
// to that number will be acked-and-dropped (Skipped=true).
func (s *Service) UnassignInboundNumber(ctx context.Context, msisdn string) error {
	return s.q.DeleteInboundNumber(ctx, strings.TrimSpace(msisdn))
}

// InboundNumber is the domain shape for inbound_numbers rows.
type InboundNumber struct {
	MSISDN    string
	TenantID  int64
	Label     string
	CreatedAt time.Time
}

func inboundFromRow(r sqlcgen.InboundMessage) *InboundMessage {
	m := &InboundMessage{
		ID:         uuid.UUID(r.ID.Bytes),
		TenantID:   r.TenantID,
		Src:        r.Src,
		Dst:        r.Dst,
		Text:       r.Text,
		ReceivedAt: r.ReceivedAt.Time,
		CreatedAt:  r.CreatedAt.Time,
	}
	if r.HorisenID != nil {
		m.HorisenID = *r.HorisenID
	}
	if r.Dcs != nil {
		m.DCS = *r.Dcs
	}
	return m
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
