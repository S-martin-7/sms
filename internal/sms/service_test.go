package sms_test

import (
	"context"
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
