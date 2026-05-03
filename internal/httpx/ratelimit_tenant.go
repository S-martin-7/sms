package httpx

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// TenantSMSLimiter is an in-memory token-bucket per tenant_id used to
// throttle expensive endpoints (POST /v1/sms, /v1/sms/bulk) so one
// noisy tenant can't starve the shared Horisen pipe. Resets on
// process restart, which is fine — the global outbox limiter still
// caps overall TPS to the provider.
//
// Default: 5 req/sec sustained, burst of 10 (i.e. ~half the global
// 10 TPS, leaving room for at least one other tenant). Override via
// SMS_PER_TENANT_TPS.
type TenantSMSLimiter struct {
	mu       sync.Mutex
	buckets  map[int64]*tenantBucket
	rps      rate.Limit
	burst    int
	idleTTL  time.Duration
}

type tenantBucket struct {
	limiter *rate.Limiter
	lastHit time.Time
}

// NewTenantSMSLimiter builds the limiter. rps<=0 falls back to 5,
// burst<=0 falls back to 2*rps. Spawns a sweeper goroutine that
// drops tenants idle for >30 min so the map stays bounded even with
// many transient tenants.
func NewTenantSMSLimiter(rps float64, burst int) *TenantSMSLimiter {
	if rps <= 0 {
		rps = 5
	}
	if burst <= 0 {
		burst = int(rps) * 2
		if burst < 1 {
			burst = 1
		}
	}
	l := &TenantSMSLimiter{
		buckets: make(map[int64]*tenantBucket),
		rps:     rate.Limit(rps),
		burst:   burst,
		idleTTL: 30 * time.Minute,
	}
	go l.sweep()
	return l
}

// Wrap is a chi middleware that pulls the tenant id from the request
// context (placed there by the APIKey middleware) and enforces the
// limit. If no tenant id is present we let the request through —
// it's the API-key middleware's job to fail unauthenticated calls.
func (l *TenantSMSLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := TenantIDFrom(r.Context())
		if tenantID == 0 {
			next.ServeHTTP(w, r)
			return
		}
		if !l.allow(tenantID) {
			// Reservation says "wait this long for the next token";
			// we surface that as a Retry-After hint.
			w.Header().Set("Retry-After", "1")
			WriteError(w, http.StatusTooManyRequests, "rate_limited",
				"per-tenant SMS rate limit exceeded; slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *TenantSMSLimiter) allow(tenantID int64) bool {
	l.mu.Lock()
	b, ok := l.buckets[tenantID]
	if !ok {
		b = &tenantBucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[tenantID] = b
	}
	b.lastHit = time.Now()
	l.mu.Unlock()
	return b.limiter.Allow()
}

func (l *TenantSMSLimiter) sweep() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-l.idleTTL)
		for id, b := range l.buckets {
			if b.lastHit.Before(cutoff) {
				delete(l.buckets, id)
			}
		}
		l.mu.Unlock()
	}
}
