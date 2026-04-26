package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/S-martin-7/sms/internal/horisen"
	"github.com/S-martin-7/sms/internal/httpx"
)

// BalanceFetcher is the narrow interface the handler needs from the
// horisen package. Lets us swap in a fake for tests.
type BalanceFetcher interface {
	Get(ctx context.Context) (*horisen.Balance, error)
}

// BalanceCache caches the most recent successful Balance response
// server-wide for `ttl`. The Horisen balance is account-level (not
// tenant-scoped), so all tenants see the same number.
type BalanceCache struct {
	fetcher BalanceFetcher
	ttl     time.Duration

	mu      sync.Mutex
	value   *horisen.Balance
	fetched time.Time
}

func NewBalanceCache(fetcher BalanceFetcher, ttl time.Duration) *BalanceCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &BalanceCache{fetcher: fetcher, ttl: ttl}
}

// Get returns a cached balance if fresh, else fetches and caches.
func (c *BalanceCache) Get(ctx context.Context) (*horisen.Balance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != nil && time.Since(c.fetched) < c.ttl {
		return c.value, nil
	}
	bal, err := c.fetcher.Get(ctx)
	if err != nil {
		return nil, err
	}
	c.value = bal
	c.fetched = time.Now()
	return bal, nil
}

type balanceResp struct {
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	UpdatedAt time.Time `json:"updated_at"`
	Cached    bool      `json:"cached"` // true when served from cache (debug aid)
}

// BalanceHandler — GET /v1/balance
//
// If the BalanceCache is nil (server was started without OAuth2 config),
// returns 503 with a clear message. This lets us deploy with the endpoint
// wired but inert until credentials are provisioned.
func BalanceHandler(cache *BalanceCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cache == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "not_configured",
				"balance endpoint disabled — set HORISEN_OAUTH_CLIENT_ID/SECRET/TOKEN_URL and HORISEN_BALANCE_URL")
			return
		}
		// Track whether we hit the cache so the client can tell.
		cache.mu.Lock()
		wasCached := cache.value != nil && time.Since(cache.fetched) < cache.ttl
		cache.mu.Unlock()

		bal, err := cache.Get(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, balanceResp{
			Amount:    bal.Amount,
			Currency:  bal.Currency,
			UpdatedAt: bal.UpdatedAt,
			Cached:    wasCached,
		})
	}
}
