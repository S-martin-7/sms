package sms_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/horisen"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// fakeSender records calls and returns scripted responses.
type fakeSender struct {
	mu      sync.Mutex
	calls   []horisen.SendParams
	next    func(p horisen.SendParams) (*horisen.SendResult, error)
	counter atomic.Int32
}

func (f *fakeSender) SendSMS(_ context.Context, p horisen.SendParams) (*horisen.SendResult, error) {
	f.counter.Add(1)
	f.mu.Lock()
	f.calls = append(f.calls, p)
	f.mu.Unlock()
	if f.next != nil {
		return f.next(p)
	}
	return &horisen.SendResult{Code: 100, Description: "OK", MsgID: "h-stub"}, nil
}

// waitStatus polls until the message reaches `want` (or fails the test).
func waitStatus(t *testing.T, svc *sms.Service, id uuid.UUID, tenantID int64, want string, timeout time.Duration) *sms.Message {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *sms.Message
	for time.Now().Before(deadline) {
		m, err := svc.GetForTenant(context.Background(), id, tenantID)
		if err == nil {
			last = m
			if m.Status == want {
				return m
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("timeout waiting for status=%q, got %q (errcode=%v)", want, last.Status, derefStr(last.ErrorCode))
	}
	t.Fatalf("timeout waiting for status=%q, message %s not found", want, id)
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestOutbox_deliversQueuedMessages(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	sender := &fakeSender{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ob := sms.NewOutbox(sms.OutboxConfig{
		Pool:     pool,
		Sender:   sender,
		TPS:      100,
		Workers:  1,
		PollIdle: 50 * time.Millisecond,
		Logger:   zerolog.Nop(),
	})
	go ob.Start(ctx)

	msg, err := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "Test", Recipient: "4179000000", Text: "hola",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := waitStatus(t, svc, msg.ID, tt.ID, "sent", 3*time.Second)
	if got.HorisenMsgID == nil || *got.HorisenMsgID != "h-stub" {
		t.Errorf("horisen_msg_id = %v, want h-stub", got.HorisenMsgID)
	}
	if sender.counter.Load() != 1 {
		t.Errorf("sender called %d times, want 1", sender.counter.Load())
	}
}

func TestOutbox_rejectsOnPermanentError(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	sender := &fakeSender{
		next: func(p horisen.SendParams) (*horisen.SendResult, error) {
			return nil, &horisen.Error{Code: 103, Description: "invalid receiver"}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ob := sms.NewOutbox(sms.OutboxConfig{
		Pool: pool, Sender: sender, TPS: 100, Workers: 1,
		PollIdle: 50 * time.Millisecond, Logger: zerolog.Nop(),
	})
	go ob.Start(ctx)

	msg, _ := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "bad", Text: "x",
	})
	got := waitStatus(t, svc, msg.ID, tt.ID, "rejected", 3*time.Second)
	if got.ErrorCode == nil || *got.ErrorCode != "103" {
		t.Errorf("error_code = %v, want 103", got.ErrorCode)
	}
}

func TestOutbox_retriesOnThrottled(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	var attempts atomic.Int32
	sender := &fakeSender{
		next: func(p horisen.SendParams) (*horisen.SendResult, error) {
			n := attempts.Add(1)
			if n == 1 {
				return nil, &horisen.Error{Code: 105, Description: "throttled"}
			}
			return &horisen.SendResult{Code: 100, MsgID: "h-after-retry"}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ob := sms.NewOutbox(sms.OutboxConfig{
		Pool: pool, Sender: sender, TPS: 100, Workers: 1,
		PollIdle:    50 * time.Millisecond,
		RetryDelays: []time.Duration{100 * time.Millisecond, 2 * time.Minute},
		Logger:      zerolog.Nop(),
	})
	go ob.Start(ctx)

	msg, _ := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "4179000000", Text: "x",
	})
	waitStatus(t, svc, msg.ID, tt.ID, "sent", 5*time.Second)
	if got := attempts.Load(); got != 2 {
		t.Errorf("sender called %d times, want 2", got)
	}
}

func TestOutbox_transportErrorRetries(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	var attempts atomic.Int32
	sender := &fakeSender{
		next: func(p horisen.SendParams) (*horisen.SendResult, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("dial tcp: boom")
			}
			return &horisen.SendResult{Code: 100, MsgID: "h-ok"}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ob := sms.NewOutbox(sms.OutboxConfig{
		Pool: pool, Sender: sender, TPS: 100, Workers: 1,
		PollIdle:    50 * time.Millisecond,
		RetryDelays: []time.Duration{100 * time.Millisecond, time.Minute},
		Logger:      zerolog.Nop(),
	})
	go ob.Start(ctx)

	msg, _ := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "4179000000", Text: "x",
	})
	waitStatus(t, svc, msg.ID, tt.ID, "sent", 5*time.Second)
}
