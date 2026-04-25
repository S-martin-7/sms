package webhooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Default backoff ladder per PLAN.md: 1m, 5m, 30m, 2h, 8h, 24h. After the
// last entry the delivery is marked `dead` and tenants must use the polling
// API to retrieve the event.
var DefaultBackoff = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
}

// DispatcherConfig wires the dispatcher worker pool.
type DispatcherConfig struct {
	Pool       *pgxpool.Pool
	Workers    int           // goroutines polling the queue
	PollIdle   time.Duration // sleep when queue is empty (default 2s)
	StaleAfter time.Duration // reset in_flight rows older than this (default 5m)
	Backoff    []time.Duration
	Timeout    time.Duration // per-attempt HTTP timeout (default 10s)
	HTTPClient *http.Client  // override for tests
	UserAgent  string        // optional
	Logger     zerolog.Logger
}

// Dispatcher pulls pending webhook_deliveries and POSTs them to tenant URLs.
type Dispatcher struct {
	cfg DispatcherConfig
	q   *sqlcgen.Queries
	hc  *http.Client
}

// NewDispatcher returns a configured dispatcher. Call Start(ctx) to begin.
func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.PollIdle <= 0 {
		cfg.PollIdle = 2 * time.Second
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 5 * time.Minute
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if len(cfg.Backoff) == 0 {
		cfg.Backoff = DefaultBackoff
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "sms-gateway-webhooks/1.0"
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Dispatcher{cfg: cfg, q: sqlcgen.New(cfg.Pool), hc: hc}
}

// Start launches workers + janitor and blocks until ctx is cancelled and
// every goroutine has returned.
func (d *Dispatcher) Start(ctx context.Context) {
	d.cfg.Logger.Info().
		Int("workers", d.cfg.Workers).
		Msg("webhook dispatcher starting")

	var wg sync.WaitGroup
	for i := 0; i < d.cfg.Workers; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			d.worker(ctx, i)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.janitor(ctx)
	}()

	wg.Wait()
	d.cfg.Logger.Info().Msg("webhook dispatcher stopped")
}

func (d *Dispatcher) worker(ctx context.Context, id int) {
	log := d.cfg.Logger.With().Int("worker", id).Logger()
	for {
		if ctx.Err() != nil {
			return
		}
		delivery, err := d.q.ClaimPendingDelivery(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if !sleepCtx(ctx, d.cfg.PollIdle) {
					return
				}
				continue
			}
			log.Warn().Err(err).Msg("claim failed")
			if !sleepCtx(ctx, d.cfg.PollIdle) {
				return
			}
			continue
		}
		d.attempt(ctx, delivery)
	}
}

// attempt fetches the endpoint, POSTs the payload, and updates the row to
// success or failed/dead based on the outcome.
func (d *Dispatcher) attempt(ctx context.Context, dlv sqlcgen.WebhookDelivery) {
	log := d.cfg.Logger.With().
		Int64("delivery_id", dlv.ID).
		Int64("endpoint_id", dlv.EndpointID).
		Int64("tenant_id", dlv.TenantID).
		Str("event_type", dlv.EventType).
		Int32("attempt", dlv.Attempts).
		Logger()

	// Look up the endpoint fresh — URL or secret might have changed.
	ep, err := d.q.GetWebhookEndpoint(ctx, sqlcgen.GetWebhookEndpointParams{
		ID: dlv.EndpointID, TenantID: dlv.TenantID,
	})
	if err != nil {
		// Endpoint deleted while delivery was queued. Mark as dead so we
		// don't keep retrying a non-existent target.
		log.Warn().Err(err).Msg("endpoint missing — marking delivery dead")
		_ = d.q.MarkDeliveryFailed(ctx, sqlcgen.MarkDeliveryFailedParams{
			ID:            dlv.ID,
			Status:        "dead",
			NextAttemptAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			LastError:     ptr(fmt.Sprintf("endpoint lookup: %v", err)),
		})
		return
	}
	if !ep.Active {
		// Skip but keep as failed in case it gets reactivated.
		log.Info().Msg("endpoint inactive — delaying delivery 1h")
		d.scheduleRetry(ctx, dlv, "endpoint inactive", "", 0, time.Hour)
		return
	}

	status, body, err := d.post(ctx, ep, dlv)
	if err != nil {
		log.Warn().Err(err).Msg("delivery transport error")
		d.scheduleRetry(ctx, dlv, err.Error(), "", 0, 0)
		return
	}

	if status >= 200 && status < 300 {
		log.Info().Int("http_status", status).Msg("delivery success")
		_ = d.q.MarkDeliverySuccess(ctx, sqlcgen.MarkDeliverySuccessParams{
			ID:           dlv.ID,
			LastStatus:   ptrInt32(int32(status)),
			LastResponse: ptr(truncate(body, 1024)),
		})
		return
	}

	// 4xx/5xx — retry per backoff.
	log.Warn().Int("http_status", status).Str("response", truncate(body, 256)).Msg("delivery non-2xx")
	d.scheduleRetry(ctx, dlv, "", body, status, 0)
}

// post executes the HTTP request to the tenant URL with the signed body.
func (d *Dispatcher) post(ctx context.Context, ep sqlcgen.WebhookEndpoint, dlv sqlcgen.WebhookDelivery) (int, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, d.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, ep.Url, bytes.NewReader(dlv.Payload))
	if err != nil {
		return 0, "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", d.cfg.UserAgent)
	req.Header.Set("X-Event-Id", uuidString(dlv.EventID))
	req.Header.Set("X-Event-Type", dlv.EventType)
	req.Header.Set(HeaderSignature, Sign(ep.Secret, dlv.Payload, time.Now()))

	resp, err := d.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(body), nil
}

// scheduleRetry computes next_attempt_at from the backoff ladder and writes
// the row as either `failed` (more attempts ahead) or `dead` (exhausted).
// `forceDelay > 0` overrides the ladder (used for e.g. inactive endpoints).
func (d *Dispatcher) scheduleRetry(ctx context.Context, dlv sqlcgen.WebhookDelivery, errStr, response string, httpStatus int, forceDelay time.Duration) {
	var (
		status string
		next   time.Time
	)
	if forceDelay > 0 {
		status = "failed"
		next = time.Now().Add(forceDelay)
	} else {
		// dlv.Attempts was already bumped by ClaimPendingDelivery, so it
		// equals the count of attempts including this one.
		idx := int(dlv.Attempts) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(d.cfg.Backoff) {
			status = "dead"
			next = time.Now()
		} else {
			status = "failed"
			next = time.Now().Add(d.cfg.Backoff[idx])
		}
	}
	params := sqlcgen.MarkDeliveryFailedParams{
		ID:            dlv.ID,
		Status:        status,
		NextAttemptAt: pgtype.Timestamptz{Time: next, Valid: true},
	}
	if errStr != "" {
		params.LastError = ptr(errStr)
	}
	if response != "" {
		params.LastResponse = ptr(truncate(response, 1024))
	}
	if httpStatus > 0 {
		params.LastStatus = ptrInt32(int32(httpStatus))
	}
	_ = d.q.MarkDeliveryFailed(ctx, params)
}

func (d *Dispatcher) janitor(ctx context.Context) {
	tick := time.NewTicker(d.cfg.StaleAfter / 2)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cutoff := time.Now().Add(-d.cfg.StaleAfter)
			if err := d.q.RecoverStaleWebhookDeliveries(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true}); err != nil {
				d.cfg.Logger.Warn().Err(err).Msg("dispatcher janitor failed")
			}
		}
	}
}

// helpers --------------------------------------------------------

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i, by := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hex[by>>4]
		out[pos+1] = hex[by&0x0f]
		pos += 2
	}
	return string(out)
}

func ptr[T any](v T) *T              { return &v }
func ptrInt32(v int32) *int32        { return &v }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

