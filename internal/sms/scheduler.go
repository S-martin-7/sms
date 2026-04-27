package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// SchedulerConfig wires the scheduler worker.
type SchedulerConfig struct {
	Pool     *pgxpool.Pool
	SMSSvc   *Service        // used to enqueue messages once a scheduled send fires
	PollIdle time.Duration   // sleep when nothing due (default 30s)
	Logger   zerolog.Logger
}

// Scheduler picks up scheduled_sends rows whose `send_at` has elapsed,
// dispatches the bulk send through the existing outbox path, and
// either re-schedules the next run (recurrent) or marks the row
// `completed` (one-shot).
type Scheduler struct {
	cfg SchedulerConfig
	q   *sqlcgen.Queries
}

func NewScheduler(cfg SchedulerConfig) *Scheduler {
	if cfg.PollIdle <= 0 {
		cfg.PollIdle = 30 * time.Second
	}
	return &Scheduler{cfg: cfg, q: sqlcgen.New(cfg.Pool)}
}

// Start runs the polling loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.cfg.Logger.Info().
		Dur("poll", s.cfg.PollIdle).
		Msg("scheduler started")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.loop(ctx)
	}()
	wg.Wait()
	s.cfg.Logger.Info().Msg("scheduler stopped")
}

func (s *Scheduler) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		// Drain everything due in this tick before sleeping. Each iteration
		// claims one row with FOR UPDATE SKIP LOCKED, so multiple workers
		// (or replicas) won't race.
		for {
			row, err := s.q.ClaimDueScheduledSend(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				break // nothing due
			}
			if err != nil {
				s.cfg.Logger.Warn().Err(err).Msg("scheduler claim failed")
				break
			}
			s.fire(ctx, row)
		}
		if !sleepCtx(ctx, s.cfg.PollIdle) {
			return
		}
	}
}

// fire executes one scheduled send: resolves recipients (explicit list or
// from a contact_list), runs EnqueueBulk, then either reschedules or
// completes the row.
func (s *Scheduler) fire(ctx context.Context, row sqlcgen.ScheduledSend) {
	log := s.cfg.Logger.With().
		Int64("scheduled_id", row.ID).
		Int64("tenant_id", row.TenantID).
		Int32("run", row.TotalRuns+1).
		Logger()

	recipients, err := s.resolveRecipients(ctx, row)
	if err != nil {
		log.Error().Err(err).Msg("scheduled: resolve recipients")
		_ = s.q.MarkScheduledSendFailed(ctx, sqlcgen.MarkScheduledSendFailedParams{
			ID: row.ID, LastError: ptr(err.Error()),
		})
		return
	}
	if len(recipients) == 0 {
		log.Warn().Msg("scheduled: 0 recipients (empty list?) — skipping but advancing schedule")
	} else {
		// Convert to EnqueueInputs and dispatch via the same path as the public API.
		inputs := make([]EnqueueInput, len(recipients))
		for i, to := range recipients {
			inputs[i] = EnqueueInput{
				TenantID: row.TenantID, Sender: row.Sender, Recipient: to, Text: row.Text,
			}
		}
		results := s.cfg.SMSSvc.EnqueueBulk(ctx, inputs)
		accepted := 0
		for _, r := range results {
			if r.Err == nil {
				accepted++
			}
		}
		log.Info().
			Int("accepted", accepted).
			Int("rejected", len(results)-accepted).
			Msg("scheduled fired")
	}

	// Compute next run.
	next := s.nextRun(row)
	status := "completed"
	if next != nil {
		status = "pending"
	}
	batchID := fmt.Sprintf("sched_%d_%d", row.ID, row.TotalRuns+1)

	var nextPg pgtype.Timestamptz
	if next != nil {
		nextPg = pgtype.Timestamptz{Time: *next, Valid: true}
	}
	if err := s.q.MarkScheduledSendFired(ctx, sqlcgen.MarkScheduledSendFiredParams{
		ID: row.ID, Status: status, SendAt: nextPg, LastBatchID: ptr(batchID),
	}); err != nil {
		log.Error().Err(err).Msg("scheduled: mark fired failed")
	}
}

// resolveRecipients returns the msisdns to send to, from either the
// explicit `recipients` JSON array or the `list_id` contact list.
func (s *Scheduler) resolveRecipients(ctx context.Context, row sqlcgen.ScheduledSend) ([]string, error) {
	if row.ListID != nil && *row.ListID > 0 {
		nums, err := s.q.GetContactListMSISDNs(ctx, sqlcgen.GetContactListMSISDNsParams{
			ListID: *row.ListID, TenantID: row.TenantID,
		})
		if err != nil {
			return nil, fmt.Errorf("read list: %w", err)
		}
		return nums, nil
	}
	if len(row.Recipients) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(row.Recipients, &out); err != nil {
		return nil, fmt.Errorf("parse recipients json: %w", err)
	}
	return out, nil
}

// nextRun computes the next send time for a recurrent schedule, or nil
// if this was a one-shot.
//
// Recurrence model (current): 'weekly' with a set of weekdays + hour/minute
// derived from the original send_at in the row's timezone. We pick the next
// listed weekday strictly after the current send_at.
func (s *Scheduler) nextRun(row sqlcgen.ScheduledSend) *time.Time {
	if row.Recurrence == nil || *row.Recurrence == "" {
		return nil
	}
	if *row.Recurrence != "weekly" || len(row.RecurrenceDays) == 0 {
		return nil
	}
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc = time.UTC
	}
	// Anchor on the row's send_at in the configured timezone.
	cur := row.SendAt.Time.In(loc)
	hour, min := cur.Hour(), cur.Minute()

	// Search up to 14 days forward to find the next allowed weekday.
	allowed := make(map[int]bool, len(row.RecurrenceDays))
	for _, d := range row.RecurrenceDays {
		allowed[int(d)] = true
	}
	candidate := time.Date(cur.Year(), cur.Month(), cur.Day(), hour, min, 0, 0, loc).
		Add(24 * time.Hour)
	for i := 0; i < 14; i++ {
		dow := int(candidate.Weekday()) // 0=Sunday..6=Saturday
		if allowed[dow] {
			t := candidate
			return &t
		}
		candidate = candidate.Add(24 * time.Hour)
	}
	return nil
}

func ptr(s string) *string { return &s }

// EnqueueScheduledSend creates a scheduled_sends row from the public API
// path. Used by /v1/sms and /v1/sms/bulk when caller passes send_at.
// Recipients is the literal list of msisdns; recurrence is one-shot.
func (s *Service) EnqueueScheduledSend(
	ctx context.Context, tenantID int64,
	sender, text string, recipients []string,
	sendAt time.Time, apiKeyID *int64, name string,
) (*sqlcgen.ScheduledSend, error) {
	if sender == "" || text == "" {
		return nil, fmt.Errorf("sender and text required")
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("recipients required")
	}
	body, _ := json.Marshal(recipients)
	row, err := s.q.CreateScheduledSend(ctx, sqlcgen.CreateScheduledSendParams{
		TenantID:   tenantID,
		Name:       ptrIfNotEmpty(name),
		Sender:     sender,
		Text:       text,
		Recipients: body,
		SendAt:     pgtype.Timestamptz{Time: sendAt, Valid: true},
		Column10:   "",
		ApiKeyID:   apiKeyID,
		// Random temp id so the rest of the code can rely on it being non-nil.
		// Used for idempotency on re-attempts (not exposed yet).
	})
	if err != nil {
		return nil, fmt.Errorf("insert scheduled: %w", err)
	}
	_ = uuid.Nil // imported intentionally for future expansion
	return &row, nil
}

func ptrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
