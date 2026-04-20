package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/S-martin-7/sms/internal/auth"
	"github.com/S-martin-7/sms/internal/httpx"
)

// AdminJWT parses an Authorization: Bearer <jwt>, verifies it with secret,
// and stores (adminID, role) in the request context.
func AdminJWT(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(hdr, "Bearer ") {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing bearer")
				return
			}
			tok := strings.TrimPrefix(hdr, "Bearer ")
			claims, err := auth.ParseJWT(secret, tok)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.SetAdmin(r.Context(), claims.Sub, claims.Role)))
		})
	}
}

// Re-exports for tests; canonical accessors live in httpx.
func AdminIDFrom(ctx context.Context) int64    { return httpx.AdminIDFrom(ctx) }
func AdminRoleFrom(ctx context.Context) string { return httpx.AdminRoleFrom(ctx) }
