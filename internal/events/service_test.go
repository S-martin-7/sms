package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/events"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
)

func newSvc(t *testing.T) (int64, *events.Service) {
	t.Helper()
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, err := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tt.ID, events.NewService(pool)
}

func TestCreate_persists(t *testing.T) {
	tenantID, svc := newSvc(t)
	e, err := svc.Create(context.Background(), tenantID, webhooks.EventSMSDelivered, map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if e.ID == 0 {
		t.Error("id should be assigned")
	}
	if e.Type != webhooks.EventSMSDelivered {
		t.Errorf("type = %q", e.Type)
	}
	var got map[string]string
	if err := json.Unmarshal(e.Payload, &got); err != nil {
		t.Fatalf("payload not json: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("payload = %v", got)
	}
}

func TestCreate_rejectsUnknownType(t *testing.T) {
	tenantID, svc := newSvc(t)
	_, err := svc.Create(context.Background(), tenantID, "sms.bogus", map[string]any{})
	if !errors.Is(err, events.ErrInvalidType) {
		t.Errorf("err = %v, want ErrInvalidType", err)
	}
}

func TestList_newestFirstAndPagination(t *testing.T) {
	tenantID, svc := newSvc(t)
	ctx := context.Background()
	// insert 5 events
	for i := 0; i < 5; i++ {
		if _, err := svc.Create(ctx, tenantID, webhooks.EventSMSDelivered, map[string]int{"i": i}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	page1, err := svc.List(ctx, tenantID, events.ListOpts{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	// Newest first
	if page1[0].ID <= page1[1].ID {
		t.Errorf("not newest-first: %d, %d", page1[0].ID, page1[1].ID)
	}

	page2, err := svc.List(ctx, tenantID, events.ListOpts{Limit: 2, CursorID: page1[1].ID})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}
	if page2[0].ID >= page1[1].ID {
		t.Errorf("page2 should start strictly older than page1 cursor")
	}

	page3, _ := svc.List(ctx, tenantID, events.ListOpts{Limit: 2, CursorID: page2[1].ID})
	if len(page3) != 1 {
		t.Errorf("page3 len = %d, want 1 (only one row left)", len(page3))
	}
}

func TestList_filterByType(t *testing.T) {
	tenantID, svc := newSvc(t)
	ctx := context.Background()
	_, _ = svc.Create(ctx, tenantID, webhooks.EventSMSDelivered, map[string]int{"i": 1})
	_, _ = svc.Create(ctx, tenantID, webhooks.EventSMSRejected, map[string]int{"i": 2})
	_, _ = svc.Create(ctx, tenantID, webhooks.EventSMSInbound, map[string]int{"i": 3})

	got, err := svc.List(ctx, tenantID, events.ListOpts{
		Types: []string{webhooks.EventSMSDelivered, webhooks.EventSMSRejected},
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	for _, e := range got {
		if e.Type != webhooks.EventSMSDelivered && e.Type != webhooks.EventSMSRejected {
			t.Errorf("unexpected type in filtered result: %q", e.Type)
		}
	}
}

func TestList_filterByTimeRange(t *testing.T) {
	tenantID, svc := newSvc(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = svc.Create(ctx, tenantID, webhooks.EventSMSDelivered, map[string]int{"i": i})
	}
	// Future "to" — everything qualifies.
	got, _ := svc.List(ctx, tenantID, events.ListOpts{To: time.Now().Add(time.Hour)})
	if len(got) != 3 {
		t.Errorf("future to: got %d, want 3", len(got))
	}
	// Past "from" — everything qualifies.
	got, _ = svc.List(ctx, tenantID, events.ListOpts{From: time.Now().Add(-time.Hour)})
	if len(got) != 3 {
		t.Errorf("past from: got %d, want 3", len(got))
	}
	// Future "from" — nothing qualifies.
	got, _ = svc.List(ctx, tenantID, events.ListOpts{From: time.Now().Add(time.Hour)})
	if len(got) != 0 {
		t.Errorf("future from: got %d, want 0", len(got))
	}
}

func TestList_tenantIsolation(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	a, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "A"})
	b, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "B"})
	svc := events.NewService(pool)

	_, _ = svc.Create(context.Background(), a.ID, webhooks.EventSMSDelivered, map[string]any{})

	got, _ := svc.List(context.Background(), b.ID, events.ListOpts{})
	if len(got) != 0 {
		t.Errorf("tenant B should see 0 events, got %d", len(got))
	}
	got, _ = svc.List(context.Background(), a.ID, events.ListOpts{})
	if len(got) != 1 {
		t.Errorf("tenant A should see 1 event, got %d", len(got))
	}
}
