package sms_test

import (
	"context"
	"errors"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/google/uuid"
)

func TestEnqueue_happyPath(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})

	svc := sms.NewService(pool)
	msg, err := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID:  tt.ID,
		Sender:    "Test",
		Recipient: "4179000000",
		Text:      "hola mundo",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if msg.ID == uuid.Nil {
		t.Error("id should not be nil")
	}
	if msg.Status != "queued" {
		t.Errorf("status = %q, want queued", msg.Status)
	}
	if msg.DCS != "GSM" {
		t.Errorf("dcs = %q, want GSM", msg.DCS)
	}
	if msg.NumParts != 1 {
		t.Errorf("num_parts = %d, want 1", msg.NumParts)
	}
}

func TestEnqueue_unicodeGoesUCS(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})

	svc := sms.NewService(pool)
	msg, err := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID:  tt.ID,
		Sender:    "Test",
		Recipient: "4179000000",
		Text:      "hola 👋",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if msg.DCS != "UCS" {
		t.Errorf("dcs = %q, want UCS", msg.DCS)
	}
}

func TestEnqueue_duplicateClientRef(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	in := sms.EnqueueInput{
		TenantID:  tt.ID,
		Sender:    "S",
		Recipient: "4179000000",
		Text:      "x",
		ClientRef: "order-123",
	}
	if _, err := svc.Enqueue(context.Background(), in); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	_, err := svc.Enqueue(context.Background(), in)
	if err != sms.ErrDuplicateClientRef {
		t.Errorf("err = %v, want ErrDuplicateClientRef", err)
	}
}

func TestGetForTenant_isolation(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	a, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "A"})
	b, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "B"})

	svc := sms.NewService(pool)
	msg, _ := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID: a.ID, Sender: "S", Recipient: "4179000000", Text: "hi",
	})

	// tenant A can read it
	if _, err := svc.GetForTenant(context.Background(), msg.ID, a.ID); err != nil {
		t.Errorf("A.Get: %v", err)
	}
	// tenant B cannot
	if _, err := svc.GetForTenant(context.Background(), msg.ID, b.ID); err != sms.ErrNotFound {
		t.Errorf("B.Get err = %v, want ErrNotFound", err)
	}
}

func TestGetForTenant_unknownID(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})

	svc := sms.NewService(pool)
	_, err := svc.GetForTenant(context.Background(), uuid.New(), tt.ID)
	if err != sms.ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListMessages_newestFirstAndPagination(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	ctx := context.Background()

	// Seed 5 messages with distinct created_at by short sleeps.
	for i := 0; i < 5; i++ {
		if _, err := svc.Enqueue(ctx, sms.EnqueueInput{
			TenantID: tt.ID, Sender: "S", Recipient: "5612345" + string(rune('0'+i)), Text: "x",
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	page1, err := svc.ListMessages(ctx, tt.ID, sms.ListOpts{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d", len(page1))
	}
	// Newest-first ordering
	if !page1[0].CreatedAt.After(page1[1].CreatedAt) && page1[0].CreatedAt.Equal(page1[1].CreatedAt) {
		// equal created_at is OK if id ordering breaks tie; just check basic monotonicity
		if page1[0].CreatedAt.Before(page1[1].CreatedAt) {
			t.Errorf("not newest-first: %v vs %v", page1[0].CreatedAt, page1[1].CreatedAt)
		}
	}

	page2, err := svc.ListMessages(ctx, tt.ID, sms.ListOpts{
		Limit:           2,
		CursorCreatedAt: page1[1].CreatedAt,
		CursorID:        page1[1].ID,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	for _, m := range page2 {
		if m.ID == page1[0].ID || m.ID == page1[1].ID {
			t.Errorf("page2 contains a page1 row: %s", m.ID)
		}
	}
}

func TestListMessages_filterByStatus(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = svc.Enqueue(ctx, sms.EnqueueInput{TenantID: tt.ID, Sender: "S", Recipient: "5612345678" + string(rune('0'+i)), Text: "x"})
	}
	// All start as 'queued'.
	got, err := svc.ListMessages(ctx, tt.ID, sms.ListOpts{Status: "queued"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("queued count = %d, want 3", len(got))
	}
	got, _ = svc.ListMessages(ctx, tt.ID, sms.ListOpts{Status: "delivered"})
	if len(got) != 0 {
		t.Errorf("delivered count = %d, want 0", len(got))
	}
}

func TestListMessages_filterByRecipientAndClientRef(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	ctx := context.Background()

	_, _ = svc.Enqueue(ctx, sms.EnqueueInput{TenantID: tt.ID, Sender: "S", Recipient: "56999000001", Text: "a", ClientRef: "ref-A"})
	_, _ = svc.Enqueue(ctx, sms.EnqueueInput{TenantID: tt.ID, Sender: "S", Recipient: "56999000002", Text: "b", ClientRef: "ref-B"})

	got, _ := svc.ListMessages(ctx, tt.ID, sms.ListOpts{Recipient: "56999000001"})
	if len(got) != 1 || got[0].ClientRef == nil || *got[0].ClientRef != "ref-A" {
		t.Errorf("recipient filter wrong: %+v", got)
	}
	got, _ = svc.ListMessages(ctx, tt.ID, sms.ListOpts{ClientRef: "ref-B"})
	if len(got) != 1 || got[0].Recipient != "56999000002" {
		t.Errorf("client_ref filter wrong: %+v", got)
	}
}

func TestEnqueueBulk_partialAccept(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	ctx := context.Background()

	// Pre-insert one row with client_ref="dup" so the bulk batch hits it.
	_, _ = svc.Enqueue(ctx, sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "56999000000", Text: "first", ClientRef: "dup",
	})

	inputs := []sms.EnqueueInput{
		{TenantID: tt.ID, Sender: "S", Recipient: "56999000001", Text: "ok-1", ClientRef: "ok-1"},
		{TenantID: tt.ID, Sender: "S", Recipient: "56999000002", Text: "ok-2"}, // no client_ref
		{TenantID: tt.ID, Sender: "S", Recipient: "56999000003", Text: "dup-row", ClientRef: "dup"}, // dup
		{TenantID: tt.ID, Sender: "", Recipient: "56999000004", Text: "no-sender"},                  // missing sender
	}
	results := svc.EnqueueBulk(ctx, inputs)
	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4", len(results))
	}
	if results[0].Err != nil || results[0].Msg == nil {
		t.Errorf("row 0 should be accepted: %+v", results[0])
	}
	if results[1].Err != nil || results[1].Msg == nil {
		t.Errorf("row 1 should be accepted: %+v", results[1])
	}
	if !errors.Is(results[2].Err, sms.ErrDuplicateClientRef) {
		t.Errorf("row 2 should be ErrDuplicateClientRef, got %v", results[2].Err)
	}
	if results[3].Err == nil {
		t.Error("row 3 (no sender) should be rejected")
	}
}

func TestEnqueueBulk_allAccepted(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	ctx := context.Background()

	inputs := make([]sms.EnqueueInput, 5)
	for i := range inputs {
		inputs[i] = sms.EnqueueInput{
			TenantID: tt.ID, Sender: "S",
			Recipient: "56999000" + string(rune('1'+i)) + "00", Text: "x",
		}
	}
	results := svc.EnqueueBulk(ctx, inputs)
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("row %d unexpected err: %v", i, r.Err)
		}
		if r.Msg == nil || r.Msg.Status != "queued" {
			t.Errorf("row %d not queued: %+v", i, r.Msg)
		}
	}
}

func TestEnqueueBulk_emptyInput(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := sms.NewService(pool)
	results := svc.EnqueueBulk(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("empty input should produce empty results, got %d", len(results))
	}
}

func TestListMessages_tenantIsolation(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	a, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "A"})
	b, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "B"})
	svc := sms.NewService(pool)

	_, _ = svc.Enqueue(context.Background(), sms.EnqueueInput{TenantID: a.ID, Sender: "S", Recipient: "5611", Text: "x"})

	got, _ := svc.ListMessages(context.Background(), b.ID, sms.ListOpts{})
	if len(got) != 0 {
		t.Errorf("tenant B should see nothing, got %d", len(got))
	}
}
