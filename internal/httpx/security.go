package httpx

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// SecurityHeaders adds the conventional hardening headers expected by
// modern browsers. CSP is intentionally restrictive: no inline script
// (Vite-built bundle is referenced by hash), no remote resources except
// Google Fonts (which the dashboard uses for typography).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// Strict-Transport-Security is also set by nginx, harmless to repeat.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// CSP only on the dashboard / admin login response. The /v1/* JSON
		// API ignores it (browsers don't honour CSP on JSON responses) but
		// it doesn't hurt to send.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data:; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// LoginRateLimiter is a sliding-window in-memory limiter scoped per IP.
// Defends /admin/login against credential stuffing and brute force without
// pulling in Redis. Window: 60s, max attempts: configurable. Trusts the
// X-Real-IP / X-Forwarded-For headers set by nginx.
type LoginRateLimiter struct {
	mu       sync.Mutex
	hits     map[string][]time.Time
	max      int
	window   time.Duration
}

func NewLoginRateLimiter(max int, window time.Duration) *LoginRateLimiter {
	if max <= 0 {
		max = 5
	}
	if window <= 0 {
		window = 60 * time.Second
	}
	rl := &LoginRateLimiter{
		hits:   make(map[string][]time.Time),
		max:    max,
		window: window,
	}
	// Background sweeper to keep the map from growing unbounded.
	go rl.sweep()
	return rl
}

// Wrap is a middleware factory that enforces the limit on the wrapped handler.
func (rl *LoginRateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.allow(ip) {
			retryAfter := int(rl.window.Seconds())
			w.Header().Set("Retry-After", itoa(retryAfter))
			WriteError(w, http.StatusTooManyRequests, "rate_limited",
				"too many login attempts; try again in a minute")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *LoginRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	hits := rl.hits[ip]
	// Drop expired hits.
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[ip] = kept
		return false
	}
	rl.hits[ip] = append(kept, now)
	return true
}

func (rl *LoginRateLimiter) sweep() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for ip, hits := range rl.hits {
			kept := hits[:0]
			for _, t := range hits {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(rl.hits, ip)
			} else {
				rl.hits[ip] = kept
			}
		}
		rl.mu.Unlock()
	}
}

// clientIP returns the best-effort source IP. Reads X-Real-IP first
// (set by nginx in the deployed config), then X-Forwarded-For first
// element, then RemoteAddr.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// Take only the first IP — others are proxies.
		for i, c := range v {
			if c == ',' {
				return trim(v[:i])
			}
		}
		return trim(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tiny helpers (avoid pulling fmt/strconv just for these)
func trim(s string) string {
	a, b := 0, len(s)
	for a < b && (s[a] == ' ' || s[a] == '\t') {
		a++
	}
	for b > a && (s[b-1] == ' ' || s[b-1] == '\t') {
		b--
	}
	return s[a:b]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
