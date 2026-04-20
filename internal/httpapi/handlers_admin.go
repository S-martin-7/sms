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
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Role      string    `json:"role"`
}

// LoginHandler validates credentials via admin.Service and issues a JWT.
func LoginHandler(svc *admin.Service, jwtSecret []byte, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in loginReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		u, err := svc.VerifyPassword(r.Context(), in.Email, in.Password)
		if err != nil {
			if errors.Is(err, admin.ErrInvalidCredentials) {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "login failed")
			return
		}
		tok, exp, err := auth.IssueJWT(jwtSecret, auth.JWTClaims{Sub: u.ID, Role: u.Role}, ttl)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "token issue failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, loginResp{Token: tok, ExpiresAt: exp, Role: u.Role})
	}
}
