package webhooks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
)

func newTenant(t *testing.T) (int64, *webhooks.Service) {
	t.Helper()
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	tt, err := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "T"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tt.ID, webhooks.NewService(pool)
}

func TestCreateEndpoint_happyPath(t *testing.T) {
	tenantID, svc := newTenant(t)
	ep, err := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tenantID,
		URL:      "https://hooks.example.com/sms",
		Events:   []string{webhooks.EventSMSDelivered, webhooks.EventSMSRejected},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ep.Secret == "" {
		t.Error("secret should be present in create response")
	}
	if !ep.Active {
		t.Error("new endpoint should be active")
	}
	if len(ep.Events) != 2 {
		t.Errorf("events = %v, want 2", ep.Events)
	}

	// GET should NOT include the secret
	got, err := svc.GetEndpoint(context.Background(), ep.ID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Secret != "" {
		t.Error("GET must not return secret")
	}
}

func TestCreateEndpoint_validation(t *testing.T) {
	tenantID, svc := newTenant(t)
	cases := []struct {
		name string
		in   webhooks.CreateEndpointInput
		want error
	}{
		{"http rejected", webhooks.CreateEndpointInput{TenantID: tenantID, URL: "http://insecure.example.com", Events: []string{webhooks.EventSMSDelivered}}, webhooks.ErrInvalidURL},
		{"empty url", webhooks.CreateEndpointInput{TenantID: tenantID, URL: "", Events: []string{webhooks.EventSMSDelivered}}, webhooks.ErrInvalidURL},
		{"no events", webhooks.CreateEndpointInput{TenantID: tenantID, URL: "https://x.example", Events: nil}, webhooks.ErrNoEvents},
		{"bad event", webhooks.CreateEndpointInput{TenantID: tenantID, URL: "https://x.example", Events: []string{"sms.exploded"}}, webhooks.ErrInvalidEvent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateEndpoint(context.Background(), c.in)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want wrapping %v", err, c.want)
			}
		})
	}
}

func TestGetEndpoint_tenantIsolation(t *testing.T) {
	pool := db.WithTestDB(t)
	ts := tenancy.NewService(pool)
	a, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "A"})
	b, _ := ts.CreateTenant(context.Background(), tenancy.CreateTenantInput{Name: "B"})
	svc := webhooks.NewService(pool)

	ep, _ := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: a.ID, URL: "https://x.example", Events: []string{webhooks.EventSMSDelivered},
	})

	if _, err := svc.GetEndpoint(context.Background(), ep.ID, b.ID); !errors.Is(err, webhooks.ErrNotFound) {
		t.Errorf("B should not see A's endpoint, got err=%v", err)
	}
}

func TestFanOut_onlySubscribedActive(t *testing.T) {
	tenantID, svc := newTenant(t)

	// Endpoint A: subscribed to delivered (target)
	a, _ := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tenantID, URL: "https://a.example", Events: []string{webhooks.EventSMSDelivered},
	})
	// Endpoint B: subscribed only to inbound (should be skipped)
	_, _ = svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tenantID, URL: "https://b.example", Events: []string{webhooks.EventSMSInbound},
	})
	// Endpoint C: subscribed to delivered but inactive
	c, _ := svc.CreateEndpoint(context.Background(), webhooks.CreateEndpointInput{
		TenantID: tenantID, URL: "https://c.example", Events: []string{webhooks.EventSMSDelivered},
	})
	if err := svc.SetActive(context.Background(), c.ID, tenantID, false); err != nil {
		t.Fatalf("set inactive: %v", err)
	}

	count, err := svc.FanOut(context.Background(), tenantID, webhooks.EventSMSDelivered, map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("fanout: %v", err)
	}
	if count != 1 {
		t.Errorf("enqueued = %d, want 1 (only A)", count)
	}
	_ = a
}

func TestFanOut_unknownEventRejected(t *testing.T) {
	tenantID, svc := newTenant(t)
	_, err := svc.FanOut(context.Background(), tenantID, "sms.gonzo", map[string]string{})
	if !errors.Is(err, webhooks.ErrInvalidEvent) {
		t.Errorf("err = %v, want ErrInvalidEvent", err)
	}
}
