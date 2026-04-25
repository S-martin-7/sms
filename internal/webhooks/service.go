package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)


// Standard event names. Tenants subscribe to any subset of these per endpoint.
const (
	EventSMSDelivered   = "sms.delivered"
	EventSMSUndelivered = "sms.undelivered"
	EventSMSRejected    = "sms.rejected"
	EventSMSInbound     = "sms.inbound"
)

// AllEvents is the set we accept on endpoint registration. Anything else
// returns ErrInvalidEvent.
var AllEvents = map[string]struct{}{
	EventSMSDelivered:   {},
	EventSMSUndelivered: {},
	EventSMSRejected:    {},
	EventSMSInbound:     {},
}

var (
	ErrNotFound      = errors.New("webhooks: endpoint not found")
	ErrInvalidURL    = errors.New("webhooks: url must be https with absolute host")
	ErrInvalidEvent  = errors.New("webhooks: unknown event type")
	ErrNoEvents      = errors.New("webhooks: at least one event required")
)

// Endpoint is the domain shape returned by the service.
type Endpoint struct {
	ID        int64
	TenantID  int64
	URL       string
	Events    []string
	Active    bool
	CreatedAt time.Time
	// Secret is only populated on creation (returned ONCE). Subsequent
	// reads leave it empty so it can never leak from the API.
	Secret string `json:"secret,omitempty"`
}

// Delivery is a row from webhook_deliveries surfaced to admin/list pages.
type Delivery struct {
	ID           int64
	EndpointID   int64
	TenantID     int64
	EventID      uuid.UUID
	EventType    string
	Status       string
	Attempts     int
	NextAttemptAt time.Time
	LastStatus   *int32
	LastError    *string
	CreatedAt    time.Time
	DeliveredAt  *time.Time
}

// Service implements endpoint CRUD and the enqueue-for-delivery path.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

// CreateEndpointInput is the payload for registering a new webhook endpoint.
type CreateEndpointInput struct {
	TenantID int64
	URL      string
	Events   []string
}

// CreateEndpoint validates input, generates a fresh HMAC secret, and persists
// the endpoint. The returned Endpoint includes the secret — surface it to the
// caller exactly once and never store it client-side.
func (s *Service) CreateEndpoint(ctx context.Context, in CreateEndpointInput) (*Endpoint, error) {
	if in.TenantID == 0 {
		return nil, fmt.Errorf("tenant_id required")
	}
	cleanURL, err := validateWebhookURL(in.URL)
	if err != nil {
		return nil, err
	}
	events, err := validateEvents(in.Events)
	if err != nil {
		return nil, err
	}
	secret, err := newSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	row, err := s.q.CreateWebhookEndpoint(ctx, sqlcgen.CreateWebhookEndpointParams{
		TenantID: in.TenantID,
		Url:      cleanURL,
		Secret:   secret,
		Events:   events,
	})
	if err != nil {
		return nil, fmt.Errorf("insert endpoint: %w", err)
	}
	ep := endpointFromRow(row)
	ep.Secret = secret
	return ep, nil
}

// GetEndpoint returns the endpoint scoped to the tenant. ErrNotFound if it
// doesn't exist or belongs to a different tenant.
func (s *Service) GetEndpoint(ctx context.Context, id, tenantID int64) (*Endpoint, error) {
	row, err := s.q.GetWebhookEndpoint(ctx, sqlcgen.GetWebhookEndpointParams{ID: id, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	return endpointFromRow(row), nil
}

// ListEndpoints returns every endpoint for the tenant (active or not).
func (s *Service) ListEndpoints(ctx context.Context, tenantID int64) ([]*Endpoint, error) {
	rows, err := s.q.ListWebhookEndpointsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}
	out := make([]*Endpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, endpointFromRow(r))
	}
	return out, nil
}

// SetActive toggles the endpoint on/off without deleting delivery history.
func (s *Service) SetActive(ctx context.Context, id, tenantID int64, active bool) error {
	return s.q.SetWebhookEndpointActive(ctx, sqlcgen.SetWebhookEndpointActiveParams{
		ID: id, TenantID: tenantID, Active: active,
	})
}

// ListDeliveriesOpts are the filters supported by the admin delivery
// listing endpoint.
type ListDeliveriesOpts struct {
	Status   string // pending|in_flight|success|failed|dead — empty = all
	CursorID int64  // BIGSERIAL id, 0 = from newest
	Limit    int    // capped to 200; 0 → 50
}

// ListDeliveriesByTenant returns webhook delivery rows for a tenant in
// newest-first order. Used by the admin "webhook deliveries" page.
func (s *Service) ListDeliveriesByTenant(ctx context.Context, tenantID int64, opts ListDeliveriesOpts) ([]Delivery, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 200 {
		opts.Limit = 200
	}
	params := sqlcgen.ListWebhookDeliveriesByTenantParams{
		TenantID: tenantID,
		CursorID: opts.CursorID,
		Lim:      int32(opts.Limit),
	}
	if opts.Status != "" {
		v := opts.Status
		params.Status = &v
	}
	rows, err := s.q.ListWebhookDeliveriesByTenant(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	out := make([]Delivery, 0, len(rows))
	for _, r := range rows {
		d := Delivery{
			ID:            r.ID,
			EndpointID:    r.EndpointID,
			TenantID:      r.TenantID,
			EventID:       uuidFromPg(r.EventID),
			EventType:     r.EventType,
			Status:        r.Status,
			Attempts:      int(r.Attempts),
			NextAttemptAt: r.NextAttemptAt.Time,
			LastStatus:    r.LastStatus,
			LastError:     r.LastError,
			CreatedAt:     r.CreatedAt.Time,
		}
		if r.DeliveredAt.Valid {
			t := r.DeliveredAt.Time
			d.DeliveredAt = &t
		}
		out = append(out, d)
	}
	return out, nil
}

// RequeueDelivery resets a delivery row to pending so the dispatcher will
// retry it on the next poll cycle. Used by the admin manual-retry button.
// Does not validate state — intentionally allows replaying success rows
// for re-delivery testing.
func (s *Service) RequeueDelivery(ctx context.Context, id int64) error {
	return s.q.RequeueWebhookDelivery(ctx, id)
}

func uuidFromPg(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	return uuid.UUID(u.Bytes)
}

// DeleteEndpoint hard-removes the endpoint and (via FK cascade) its deliveries.
func (s *Service) DeleteEndpoint(ctx context.Context, id, tenantID int64) error {
	return s.q.DeleteWebhookEndpoint(ctx, sqlcgen.DeleteWebhookEndpointParams{ID: id, TenantID: tenantID})
}

// FanOut enqueues one delivery per active endpoint of `tenantID` that is
// subscribed to `eventType`. Returns the count of deliveries enqueued. The
// payload should be a JSON-serialisable struct matching the wire format
// tenants will receive (the dispatcher signs whatever bytes it sends).
func (s *Service) FanOut(ctx context.Context, tenantID int64, eventType string, payload any) (int, error) {
	if _, ok := AllEvents[eventType]; !ok {
		return 0, fmt.Errorf("%w: %s", ErrInvalidEvent, eventType)
	}
	endpoints, err := s.q.ListActiveEndpointsForEvent(ctx, sqlcgen.ListActiveEndpointsForEventParams{
		TenantID:  tenantID,
		EventType: eventType,
	})
	if err != nil {
		return 0, fmt.Errorf("list endpoints: %w", err)
	}
	if len(endpoints) == 0 {
		return 0, nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	count := 0
	for _, ep := range endpoints {
		eventID := uuid.New()
		_, err := s.q.EnqueueWebhookDelivery(ctx, sqlcgen.EnqueueWebhookDeliveryParams{
			EndpointID: ep.ID,
			TenantID:   tenantID,
			EventID:    pgtype.UUID{Bytes: eventID, Valid: true},
			EventType:  eventType,
			Payload:    body,
		})
		if err != nil {
			// Don't fail the whole fan-out for one bad row; the others may
			// still be deliverable. Log via caller.
			return count, fmt.Errorf("enqueue delivery for endpoint %d: %w", ep.ID, err)
		}
		count++
	}
	return count, nil
}

// validateWebhookURL ensures the URL is absolute and uses HTTPS so we never
// send signed events over plain HTTP.
func validateWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%w: scheme=%q (must be https)", ErrInvalidURL, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return u.String(), nil
}

// validateEvents dedupes the input list and rejects unknown events.
func validateEvents(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, ErrNoEvents
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, e := range in {
		e = strings.TrimSpace(e)
		if _, ok := AllEvents[e]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidEvent, e)
		}
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

// newSecret generates a 32-byte URL-safe-ish hex secret. Plenty of entropy
// for HMAC and easy for tenants to copy/paste from a UI.
func newSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(buf), nil
}

func endpointFromRow(r sqlcgen.WebhookEndpoint) *Endpoint {
	return &Endpoint{
		ID:        r.ID,
		TenantID:  r.TenantID,
		URL:       r.Url,
		Events:    append([]string{}, r.Events...),
		Active:    r.Active,
		CreatedAt: r.CreatedAt.Time,
	}
}
