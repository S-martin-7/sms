package sms_test

import (
	"context"
	"testing"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestParseDLR_realPayloadShape(t *testing.T) {
	body := []byte(`{"accountName":"16602_SamuelOTP","custom":{"msgId":"2dcb0a75-961c-41c8-843d-fa8a60f7c75a","tenantId":1},"dlrTime":1,"errorMessage":"No error","event":"DELIVERED","msgId":"0d577919-4001-ef2a-807d-b56c1311492b","numParts":1,"partNum":0,"sendTime":0}`)
	d, err := sms.ParseDLR(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Event != "DELIVERED" {
		t.Errorf("event = %q", d.Event)
	}
	if d.Custom.MsgID != "2dcb0a75-961c-41c8-843d-fa8a60f7c75a" {
		t.Errorf("custom.msgId = %q", d.Custom.MsgID)
	}
	if d.HorisenMsgID != "0d577919-4001-ef2a-807d-b56c1311492b" {
		t.Errorf("msgId = %q", d.HorisenMsgID)
	}
	if d.NumParts != 1 || d.PartNum != 0 {
		t.Errorf("parts = %d/%d", d.PartNum, d.NumParts)
	}
}

func TestParseDLR_rejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"not json":            `not json`,
		"no event":            `{"custom":{"msgId":"x"}}`,
		"no msgId nor custom": `{"event":"DELIVERED"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sms.ParseDLR([]byte(body)); err == nil {
				t.Errorf("expected error for %q", body)
			}
		})
	}
}

func TestDLRStatusFor(t *testing.T) {
	cases := []struct {
		event     string
		want      string
		shouldMap bool
	}{
		{"DELIVERED", "delivered", true},
		{"UNDELIVERED", "undelivered", true},
		{"EXPIRED", "undelivered", true},
		{"REJECTED", "rejected", true},
		{"BUFFERED", "", false},
		{"WHATEVER", "", false},
	}
	for _, c := range cases {
		got, ok := sms.DLRStatusFor(c.event)
		if ok != c.shouldMap || got != c.want {
			t.Errorf("DLRStatusFor(%q) = (%q,%v); want (%q,%v)", c.event, got, ok, c.want, c.shouldMap)
		}
	}
}

func TestApplyDLR_transitionsSentToDelivered(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	msg, err := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "4179000000", Text: "hi",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// Force the row into 'sent' state directly so we don't need a real Horisen.
	q := sqlcgen.New(pool)
	hMsg := "horisen-abc"
	if err := q.MarkMessageSent(context.Background(), sqlcgen.MarkMessageSentParams{
		ID:           pgtype.UUID{Bytes: msg.ID, Valid: true},
		HorisenMsgID: &hMsg,
	}); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	dlr := &sms.DLR{
		HorisenMsgID: hMsg,
		Event:        "DELIVERED",
		ErrorMessage: "No error",
	}
	dlr.Custom.MsgID = msg.ID.String()
	dlr.Custom.TenantID = tt.ID

	res, err := svc.ApplyDLR(context.Background(), dlr)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected transition, got skipped: %s", res.SkipReason)
	}
	if res.NewStatus != "delivered" {
		t.Errorf("new status = %q, want delivered", res.NewStatus)
	}

	// Verify persisted state
	got, _ := svc.GetForTenant(context.Background(), msg.ID, tt.ID)
	if got.Status != "delivered" {
		t.Errorf("persisted status = %q, want delivered", got.Status)
	}
	if got.FinalAt == nil {
		t.Error("final_at should be set")
	}
}

func TestApplyDLR_idempotentForLateDuplicate(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	msg, _ := svc.Enqueue(context.Background(), sms.EnqueueInput{
		TenantID: tt.ID, Sender: "S", Recipient: "4179000000", Text: "hi",
	})
	q := sqlcgen.New(pool)
	hMsg := "horisen-xyz"
	_ = q.MarkMessageSent(context.Background(), sqlcgen.MarkMessageSentParams{
		ID: pgtype.UUID{Bytes: msg.ID, Valid: true}, HorisenMsgID: &hMsg,
	})

	dlr := &sms.DLR{HorisenMsgID: hMsg, Event: "DELIVERED"}
	dlr.Custom.MsgID = msg.ID.String()

	// First DLR transitions to delivered.
	if _, err := svc.ApplyDLR(context.Background(), dlr); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Second DLR with the same body (or a late REJECTED) should not clobber.
	dlr.Event = "REJECTED"
	res, err := svc.ApplyDLR(context.Background(), dlr)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skip on terminal-state row, got new=%q", res.NewStatus)
	}
	got, _ := svc.GetForTenant(context.Background(), msg.ID, tt.ID)
	if got.Status != "delivered" {
		t.Errorf("status mutated by late DLR: %q", got.Status)
	}
}

func TestApplyDLR_unknownMessage(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := sms.NewService(pool)
	dlr := &sms.DLR{HorisenMsgID: "nonexistent", Event: "DELIVERED"}
	dlr.Custom.MsgID = "00000000-0000-0000-0000-000000000000"
	_, err := svc.ApplyDLR(context.Background(), dlr)
	if err != sms.ErrDLRMessageNotFound {
		t.Errorf("err = %v, want ErrDLRMessageNotFound", err)
	}
}

func TestApplyDLR_skipsNonTerminalEvents(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := sms.NewService(pool)
	dlr := &sms.DLR{Event: "BUFFERED"}
	dlr.Custom.MsgID = "00000000-0000-0000-0000-000000000000"
	res, err := svc.ApplyDLR(context.Background(), dlr)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Skipped {
		t.Error("BUFFERED should be skipped")
	}
}
