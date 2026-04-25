package webhooks_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/rs/zerolog"
)

func TestDispatcher_deliversToTenantAndSigns(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := webhooks.NewService(pool)

	var (
		gotBody []byte
		gotSig  string
		gotEvtH string
		hits    atomic.Int32
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		gotSig = r.Header.Get(webhooks.HeaderSignature)
		gotEvtH = r.Header.Get("X-Event-Id")
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ep, err := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tt.ID,
		URL:      srv.URL,
		Events:   []string{webhooks.EventSMSDelivered},
	})
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	secret := ep.Secret

	payload := map[string]any{"type": "sms.delivered", "msg_id": "abc"}
	if _, err := svc.FanOut(context.Background(), tt.ID, webhooks.EventSMSDelivered, payload); err != nil {
		t.Fatalf("fanout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disp := webhooks.NewDispatcher(webhooks.DispatcherConfig{
		Pool:       pool,
		Workers:    1,
		PollIdle:   50 * time.Millisecond,
		HTTPClient: srv.Client(), // accepts the test server's TLS cert
		Logger:     zerolog.Nop(),
	})
	go disp.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && hits.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
	if gotSig == "" {
		t.Error("X-Signature header missing")
	}
	if err := webhooks.Verify(secret, gotSig, gotBody, time.Now(), webhooks.MaxClockSkew); err != nil {
		t.Errorf("signature did not verify: %v", err)
	}
	if gotEvtH == "" {
		t.Error("X-Event-Id header missing")
	}

	// Verify the body matches the payload we enqueued.
	var got map[string]any
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if got["msg_id"] != "abc" {
		t.Errorf("body = %v", got)
	}
}

func TestDispatcher_retriesOn5xxThenSucceeds(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := webhooks.NewService(pool)

	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _ = svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tt.ID, URL: srv.URL, Events: []string{webhooks.EventSMSDelivered},
	})
	if _, err := svc.FanOut(context.Background(), tt.ID, webhooks.EventSMSDelivered, "x"); err != nil {
		t.Fatalf("fanout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disp := webhooks.NewDispatcher(webhooks.DispatcherConfig{
		Pool:       pool,
		Workers:    1,
		PollIdle:   25 * time.Millisecond,
		HTTPClient: srv.Client(),
		Backoff:    []time.Duration{50 * time.Millisecond, 50 * time.Millisecond},
		Logger:     zerolog.Nop(),
	})
	go disp.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && hits.Load() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	if hits.Load() < 2 {
		t.Fatalf("expected at least 2 hits, got %d", hits.Load())
	}
}

func TestDispatcher_marksDeadAfterBackoffExhausted(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := webhooks.NewService(pool)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ep, _ := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tt.ID, URL: srv.URL, Events: []string{webhooks.EventSMSDelivered},
	})
	if _, err := svc.FanOut(context.Background(), tt.ID, webhooks.EventSMSDelivered, "x"); err != nil {
		t.Fatalf("fanout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disp := webhooks.NewDispatcher(webhooks.DispatcherConfig{
		Pool:       pool,
		Workers:    1,
		PollIdle:   20 * time.Millisecond,
		HTTPClient: srv.Client(),
		// 2 attempts then dead — short ladder for the test.
		Backoff: []time.Duration{20 * time.Millisecond, 20 * time.Millisecond},
		Logger:  zerolog.Nop(),
	})
	go disp.Start(ctx)

	q := sqlcgen.New(pool)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := q.ListDeliveriesForEndpoint(context.Background(), sqlcgen.ListDeliveriesForEndpointParams{
			EndpointID: ep.ID, TenantID: tt.ID, Limit: 1,
		})
		if err == nil && len(rows) == 1 && rows[0].Status == "dead" {
			cancel()
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	cancel()
	t.Fatal("delivery never reached dead status")
}
