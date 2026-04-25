package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
)

// HorisenCallbackAuthConfig holds the credentials the Horisen DLR/MO endpoints
// will accept. At least one of (User+Pass) or QuerySecret must be configured;
// the request is allowed if EITHER mode validates.
type HorisenCallbackAuthConfig struct {
	BasicUser   string // matched against HTTP Basic Auth username
	BasicPass   string // matched against HTTP Basic Auth password
	QuerySecret string // matched against the ?sig= query parameter
}

// HorisenCallbackAuth returns middleware that accepts a request signed with
// EITHER HTTP Basic Auth OR a shared-secret query string (?sig=...). Horisen
// historically uses ?sig= and some panels still default to it; the Basic Auth
// path supports the newer panel option.
//
// Fails closed: if no auth mode is configured, every request gets 401.
func HorisenCallbackAuth(realm string, cfg HorisenCallbackAuthConfig) func(http.Handler) http.Handler {
	expectedUser := []byte(cfg.BasicUser)
	expectedPass := []byte(cfg.BasicPass)
	expectedSig := []byte(cfg.QuerySecret)
	basicConfigured := len(expectedUser) > 0 && len(expectedPass) > 0
	sigConfigured := len(expectedSig) > 0
	configured := basicConfigured || sigConfigured

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !configured {
				unauthorized(w, realm)
				return
			}
			if sigConfigured {
				if sig := r.URL.Query().Get("sig"); sig != "" &&
					subtle.ConstantTimeCompare([]byte(sig), expectedSig) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			if basicConfigured {
				if u, p, ok := r.BasicAuth(); ok &&
					subtle.ConstantTimeCompare([]byte(u), expectedUser) == 1 &&
					subtle.ConstantTimeCompare([]byte(p), expectedPass) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			unauthorized(w, realm)
		})
	}
}

// BasicAuth is a thin wrapper kept for callers that only want Basic Auth.
func BasicAuth(realm, user, pass string) func(http.Handler) http.Handler {
	return HorisenCallbackAuth(realm, HorisenCallbackAuthConfig{
		BasicUser: user,
		BasicPass: pass,
	})
}

func unauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "basic auth or ?sig= required")
}
