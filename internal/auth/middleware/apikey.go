package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/tenancy"
)

// APIKey verifies the X-API-Key header against the tenancy service and
// injects the tenant id into the request context.
func APIKey(svc *tenancy.Service, pepper string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-API-Key")
			if token == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing X-API-Key")
				return
			}
			tenantID, err := svc.VerifyAPIKey(r.Context(), token, pepper)
			if err != nil {
				switch {
				case errors.Is(err, tenancy.ErrTenantSuspended):
					httpx.WriteError(w, http.StatusForbidden, "tenant_suspended",
						"tenant account is suspended; contact support")
				case errors.Is(err, tenancy.ErrAPIKeyInvalid),
					errors.Is(err, tenancy.ErrAPIKeyNotFound),
					errors.Is(err, tenancy.ErrAPIKeyRevoked):
					httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid api key")
				default:
					httpx.WriteError(w, http.StatusInternalServerError, "internal", "auth lookup failed")
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.SetTenantID(r.Context(), tenantID)))
		})
	}
}

// TenantIDFrom is re-exported for tests; the canonical accessor lives in httpx.
func TenantIDFrom(ctx context.Context) int64 { return httpx.TenantIDFrom(ctx) }
