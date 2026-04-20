package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func TestAPIKeyMiddleware(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()
	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "T1"})
	issued, _ := svc.IssueAPIKey(ctx, tt.ID, "test", "pepper-mw")

	handler := middleware.APIKey(svc, "pepper-mw")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := middleware.TenantIDFrom(r.Context()); got != tt.ID {
				t.Errorf("ctx tenantID = %d, want %d", got, tt.ID)
			}
			w.WriteHeader(204)
		}))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", issued.Token, 204},
		{"missing", "", 401},
		{"malformed", "garbage", 401},
		{"unknown", "sk_live_"+"0000000000000000000000000000000000000000000", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				req.Header.Set("X-API-Key", c.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("code = %d, want %d", rec.Code, c.want)
			}
		})
	}

	_ = svc.RevokeAPIKey(ctx, issued.Record.ID)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", issued.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("after revoke code = %d, want 401", rec.Code)
	}
}
