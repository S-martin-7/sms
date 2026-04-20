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

func waitStatus(t *testing.T, svc *sms.Service, id interface {
	String() string
}, tenantID int64, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m, err := svc.GetForTenant(context.Background(), parseUUID(t, id.String()), tenantID)
		if err == nil && m.Status == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	m, _ := svc.GetForTenant(context.Background(), parseUUID(t, id.String()), tenantID)
	if m != nil {
		t.Fatalf("timeout waiting for status=%q, got %q (errcode=%v)", want, m.Status, deref(m.ErrorCode))
	}
	t.Fatalf("timeout waiting for status=%q, message not found", want)
}

func deref(s *string) string {
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

	waitStatus(t, svc, msg.ID, tt.ID, "sent", 3*time.Second)
	if sender.counter.Load() != 1 {
		t.Errorf("sender called %d times, want 1", sender.counter.Load())
	}
	sender.mu.Lock()
	if len(sender.calls) != 1 || sender.calls[0].Receiver != "4179000000" {
		t.Errorf("unexpected call: %+v", sender.calls)
	}
	sender.mu.Unlock()
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
	waitStatus(t, svc, msg.ID, tt.ID, "rejected", 3*time.Second)

	got, _ := svc.GetForTenant(ctx, parseUUID(t, msg.ID.String()), tt.ID)
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

	// Use 100ms first retry so test doesn't sit for 30s.
	ob := sms.NewOutbox(sms.OutboxConfig{
		Pool: pool, Sender: sender, TPS: 100, Workers: 1,
		PollIdle: 50 * time.Millisecond,
		RetryDelays: []time.Duration{
			100 * time.Millisecond,
			2 * time.Minute,
		},
		Logger: zerolog.Nop(),
	})
	go ob.Start(ctx)

	msg, _ := svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "4179000000", Text: "x",
	})
	waitStatus(t, svc, msg.ID, tt.ID, "sent", 5*time.Second)

	if got := attempts.Load(); got != 2 {
		t.Errorf("sender called %d times, want 2 (1 fail + 1 success)", got)
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
