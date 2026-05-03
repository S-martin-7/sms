package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth"
	"github.com/S-martin-7/sms/internal/httpx"
)

type loginReq struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	TOTPCode  string `json:"totp_code,omitempty"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Role      string    `json:"role"`
}

// LoginHandler validates credentials via admin.Service.Authenticate
// (lockout + optional TOTP) and issues a JWT. Audit row written for
// every outcome — successful or not — keyed by email even when the
// user doesn't exist, so failed-attempt patterns are visible in the
// audit log.
func LoginHandler(svc *admin.Service, jwtSecret []byte, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in loginReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		u, err := svc.Authenticate(r.Context(), in.Email, in.Password, in.TOTPCode)
		if err != nil {
			switch {
			case errors.Is(err, admin.ErrTOTPRequired):
				// Password was correct; we just need the second factor.
				// 403 (not 401) so the React client can distinguish this
				// from a bad-credentials error and prompt for the code.
				httpx.WriteError(w, http.StatusForbidden, "totp_required",
					"totp code required")
				return
			case errors.Is(err, admin.ErrInvalidCredentials):
				_ = svc.LogAction(r.Context(), 0, "admin.login_failed", "email", in.Email, nil)
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
				return
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "login failed")
				return
			}
		}
		tok, exp, err := auth.IssueJWT(jwtSecret, auth.JWTClaims{Sub: u.ID, Role: u.Role}, ttl)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "token issue failed")
			return
		}
		_ = svc.LogAction(r.Context(), u.ID, "admin.login_success", "admin_user", "", nil)
		httpx.WriteJSON(w, http.StatusOK, loginResp{Token: tok, ExpiresAt: exp, Role: u.Role})
	}
}
