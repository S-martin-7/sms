package sms_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func TestParseMO_acceptsBothIdAndMsgId(t *testing.T) {
	cases := map[string][]byte{
		"id field":    []byte(`{"id":"abc","src":"56999","dst":"56123","text":"hi"}`),
		"msgId field": []byte(`{"msgId":"abc","src":"56999","dst":"56123","text":"hi"}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := sms.ParseMO(body)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if m.Source != "56999" || m.Dest != "56123" || m.Text != "hi" {
				t.Errorf("fields: src=%q dst=%q text=%q", m.Source, m.Dest, m.Text)
			}
		})
	}
}

func TestParseMO_rejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"not json":  `not json`,
		"no src":    `{"dst":"x","text":"hi"}`,
		"no dst":    `{"src":"x","text":"hi"}`,
		"no text":   `{"src":"a","dst":"b"}`,
		"empty src": `{"src":"","dst":"b","text":"hi"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sms.ParseMO([]byte(body)); err == nil {
				t.Errorf("expected error for %q", body)
			}
		})
	}
}

func TestApplyMO_routesToTenantViaMSISDN(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	if _, err := svc.AssignInboundNumber(context.Background(), "56123456789", tt.ID, "test number"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	mo := &sms.MO{
		HorisenID: "horisen-mo-1",
		Source:    "56999111222",
		Dest:      "56123456789",
		Text:      "ping",
	}
	res, err := svc.ApplyMO(context.Background(), mo)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Skipped {
		t.Fatalf("should not skip — number is mapped: %s", res.SkipReason)
	}
	if res.Inbound.TenantID != tt.ID {
		t.Errorf("tenant_id = %d, want %d", res.Inbound.TenantID, tt.ID)
	}
	if res.Inbound.Text != "ping" {
		t.Errorf("text = %q", res.Inbound.Text)
	}
}

func TestApplyMO_skipsWhenNoRoute(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := sms.NewService(pool)

	mo := &sms.MO{Source: "56999", Dest: "doesnotexist", Text: "ping"}
	res, err := svc.ApplyMO(context.Background(), mo)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skip for unmapped dst")
	}
}

func TestApplyMO_idempotentOnHorisenID(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)
	_, _ = svc.AssignInboundNumber(context.Background(), "56123456789", tt.ID, "")

	mo := &sms.MO{HorisenID: "dedupe-key", Source: "56999", Dest: "56123456789", Text: "hello"}
	r1, err := svc.ApplyMO(context.Background(), mo)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	r2, err := svc.ApplyMO(context.Background(), mo)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if r1.Inbound.ID != r2.Inbound.ID {
		t.Errorf("expected same id on duplicate horisen_id, got %s vs %s", r1.Inbound.ID, r2.Inbound.ID)
	}
}

func TestAssignAndUnassignInboundNumber(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	svc := sms.NewService(pool)

	if _, err := svc.AssignInboundNumber(context.Background(), "56999000000", tt.ID, "marketing"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	rows, err := svc.ListInboundNumbers(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].MSISDN != "56999000000" || rows[0].Label != "marketing" {
		t.Errorf("rows = %+v", rows)
	}

	if err := svc.UnassignInboundNumber(context.Background(), "56999000000"); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	rows, _ = svc.ListInboundNumbers(context.Background())
	if len(rows) != 0 {
		t.Errorf("after unassign rows = %+v", rows)
	}
}
