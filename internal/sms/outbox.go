package sms

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/horisen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// Sender is the narrow dependency the outbox needs from the Horisen client.
// Satisfied by *horisen.Client, and easy to fake in tests.
type Sender interface {
	SendSMS(ctx context.Context, p horisen.SendParams) (*horisen.SendResult, error)
}

// OutboxConfig wires the outbox worker pool.
type OutboxConfig struct {
	Pool        *pgxpool.Pool
	Sender      Sender
	TPS         int                   // Horisen's contracted throughput — shared limiter
	Workers     int                   // how many goroutines poll the queue
	DLRURL      string                // full URL incl ?sig= for Horisen to POST DLRs to
	PollIdle    time.Duration         // how long to wait when the queue is empty (default 1s)
	RetryDelays []time.Duration       // backoff ladder for retryable failures (default 30s,2m,10m,30m,2h)
	StaleAfter  time.Duration         // reclaim `sending` rows older than this (default 5m)
	Logger      zerolog.Logger
}

// Outbox drains the `messages` queue and submits each one to Horisen.
type Outbox struct {
	cfg     OutboxConfig
	q       *sqlcgen.Queries
	limiter *rate.Limiter
}

// NewOutbox returns a configured outbox. Call Start(ctx) to begin processing.
func NewOutbox(cfg OutboxConfig) *Outbox {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.TPS <= 0 {
		cfg.TPS = 10
	}
	if cfg.PollIdle <= 0 {
		cfg.PollIdle = time.Second
	}
	if len(cfg.RetryDelays) == 0 {
		cfg.RetryDelays = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 5 * time.Minute
	}
	lim := rate.NewLimiter(rate.Limit(cfg.TPS), cfg.TPS)
	return &Outbox{cfg: cfg, q: sqlcgen.New(cfg.Pool), limiter: lim}
}

// Start spawns the worker pool + a stale-sending janitor. Blocks until ctx
// is cancelled and every goroutine has returned — important for tests that
// reuse the same Postgres DB between test functions.
func (o *Outbox) Start(ctx context.Context) {
	o.cfg.Logger.Info().
		Int("workers", o.cfg.Workers).
		Int("tps", o.cfg.TPS).
		Msg("outbox starting")

	var wg sync.WaitGroup
	for i := 0; i < o.cfg.Workers; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			o.worker(ctx, i)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.janitor(ctx)
	}()

	wg.Wait()
	o.cfg.Logger.Info().Msg("outbox stopped")
}

func (o *Outbox) worker(ctx context.Context, id int) {
	log := o.cfg.Logger.With().Int("worker", id).Logger()
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := o.q.ClaimQueuedMessage(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if !sleepCtx(ctx, o.cfg.PollIdle) {
					return
				}
				continue
			}
			log.Warn().Err(err).Msg("claim failed")
			if !sleepCtx(ctx, o.cfg.PollIdle) {
				return
			}
			continue
		}
		if err := o.limiter.Wait(ctx); err != nil {
			// ctx cancelled — the janitor will reclaim this row.
			return
		}
		o.deliver(ctx, msg)
	}
}

func (o *Outbox) deliver(ctx context.Context, msg sqlcgen.Message) {
	log := o.cfg.Logger.With().
		Str("msg_id", fmtUUID(msg.ID)).
		Int64("tenant_id", msg.TenantID).
		Str("recipient", msg.Recipient).
		Int32("attempt", msg.Attempts).
		Logger()

	params := horisen.SendParams{
		Sender:   msg.Sender,
		Receiver: msg.Recipient,
		Text:     msg.Text,
		DCS:      horisen.DCS(msg.Dcs),
		DLRMask:  19, // delivered + undelivered + rejected
		DLRURL:   o.cfg.DLRURL,
		Custom: map[string]any{
			"tenantId": msg.TenantID,
			"msgId":    fmtUUID(msg.ID),
		},
	}

	result, err := o.cfg.Sender.SendSMS(ctx, params)
	if err != nil {
		var hErr *horisen.Error
		if errors.As(err, &hErr) {
			if horisen.IsRetryable(hErr.Code) {
				o.retry(ctx, msg, int(msg.Attempts), "horisen", hErr.Error())
				log.Warn().Err(err).Msg("horisen retryable, requeued")
				return
			}
			code := fmt.Sprintf("%d", hErr.Code)
			o.reject(ctx, msg.ID, &code, strPtr(hErr.Description))
			log.Error().Err(err).Msg("horisen permanent, rejected")
			return
		}
		// transport / 5xx — retry
		o.retry(ctx, msg, int(msg.Attempts), "transport", err.Error())
		log.Warn().Err(err).Msg("transport error, requeued")
		return
	}

	var hmsg *string
	if result != nil && result.MsgID != "" {
		v := result.MsgID
		hmsg = &v
	}
	if err := o.q.MarkMessageSent(ctx, sqlcgen.MarkMessageSentParams{
		ID:           msg.ID,
		HorisenMsgID: hmsg,
	}); err != nil {
		log.Error().Err(err).Msg("mark-sent failed — message may be reprocessed")
		return
	}
	log.Info().Str("horisen_msg_id", deref(hmsg)).Msg("sent")
}

func (o *Outbox) retry(ctx context.Context, msg sqlcgen.Message, attempt int, code, desc string) {
	delay := o.backoffFor(attempt)
	if delay < 0 {
		// exceeded ladder — give up permanently
		c := code
		d := desc
		o.reject(ctx, msg.ID, &c, &d)
		return
	}
	next := time.Now().Add(delay)
	_ = o.q.BumpMessageRetry(ctx, sqlcgen.BumpMessageRetryParams{
		ID:            msg.ID,
		NextAttemptAt: pgtype.Timestamptz{Time: next, Valid: true},
		ErrorCode:     strPtr(code),
		ErrorMessage:  strPtr(desc),
	})
}

func (o *Outbox) reject(ctx context.Context, id pgtype.UUID, code, desc *string) {
	_ = o.q.MarkMessageRejected(ctx, sqlcgen.MarkMessageRejectedParams{
		ID:           id,
		ErrorCode:    code,
		ErrorMessage: desc,
	})
}

// backoffFor returns the delay for the Nth retry (1-indexed), or -1 if
// we've exhausted the ladder.
func (o *Outbox) backoffFor(attempt int) time.Duration {
	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(o.cfg.RetryDelays) {
		return -1
	}
	return o.cfg.RetryDelays[idx]
}

// janitor periodically resets rows stuck in `sending` beyond StaleAfter.
// Handles crashes where a worker claimed a message but never finished.
func (o *Outbox) janitor(ctx context.Context) {
	tick := time.NewTicker(o.cfg.StaleAfter / 2)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cutoff := time.Now().Add(-o.cfg.StaleAfter)
			if err := o.q.RecoverStaleSending(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true}); err != nil {
				o.cfg.Logger.Warn().Err(err).Msg("janitor recover failed")
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

func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
