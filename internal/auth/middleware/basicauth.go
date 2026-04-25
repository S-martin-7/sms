package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
)

// BasicAuth returns middleware that requires HTTP Basic Auth matching the
// configured user/password. Used to protect Horisen DLR/MO callback endpoints.
//
// If either user or password is empty, every request is rejected with 401 —
// fail closed so a misconfigured deploy never leaves the endpoint open.
func BasicAuth(realm, user, pass string) func(http.Handler) http.Handler {
	expectedUser := []byte(user)
	expectedPass := []byte(pass)
	configured := len(expectedUser) > 0 && len(expectedPass) > 0
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !configured {
				unauthorized(w, realm)
				return
			}
			u, p, ok := r.BasicAuth()
			if !ok {
				unauthorized(w, realm)
				return
			}
			userOK := subtle.ConstantTimeCompare([]byte(u), expectedUser) == 1
			passOK := subtle.ConstantTimeCompare([]byte(p), expectedPass) == 1
			if !(userOK && passOK) {
				unauthorized(w, realm)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "basic auth required")
}
