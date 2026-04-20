package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/S-martin-7/sms/internal/auth"
	"github.com/S-martin-7/sms/internal/auth/middleware"
)

func TestAdminJWTMiddleware(t *testing.T) {
	secret := []byte("jwt-mw-secret")
	tok, _, _ := auth.IssueJWT(secret, auth.JWTClaims{Sub: 7, Role: "superadmin"}, time.Hour)

	handler := middleware.AdminJWT(secret)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, role := middleware.AdminIDFrom(r.Context()), middleware.AdminRoleFrom(r.Context())
			if id != 7 || role != "superadmin" {
				t.Errorf("ctx id/role = %d/%q", id, role)
			}
			w.WriteHeader(204)
		}))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer " + tok, 204},
		{"missing", "", 401},
		{"no-prefix", tok, 401},
		{"bad", "Bearer garbage", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("code = %d, want %d", rec.Code, c.want)
			}
		})
	}
}
