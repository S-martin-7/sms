package horisen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthConfig holds the credentials and URL for Horisen's client_credentials
// flow. Used by API endpoints that aren't authenticated with the basic
// username/password (Balance API, customer info, etc.).
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	TokenURL     string         // e.g. https://accounts.horisen.com/oauth2/access-token
	HTTPClient   *http.Client   // optional override (tests)
}

// TokenCache holds an in-memory access token with proactive refresh at
// 80% of its TTL. Safe for concurrent callers — they share a single
// in-flight refresh via the mutex.
type TokenCache struct {
	cfg OAuthConfig

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewTokenCache returns a cache wired to the given OAuth config.
// The first Get(ctx) call fetches a fresh token.
func NewTokenCache(cfg OAuthConfig) *TokenCache {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &TokenCache{cfg: cfg}
}

// tokenResponse is the standard OAuth2 client_credentials response shape.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // seconds
}

// Get returns a valid access token, refreshing if needed. We refresh when
// less than 20% of the TTL remains so callers never trip on a token that
// just expired between getting it and using it.
func (c *TokenCache) Get(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Until(c.expiresAt) > 0 {
		return c.token, nil
	}
	return c.refreshLocked(ctx)
}

func (c *TokenCache) refreshLocked(ctx context.Context) (string, error) {
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" || c.cfg.TokenURL == "" {
		return "", errors.New("horisen oauth: ClientID, ClientSecret and TokenURL required")
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth: do request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("oauth: token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("oauth: decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("oauth: empty access_token in response")
	}
	if tok.ExpiresIn <= 0 {
		// Spec doesn't require expires_in; fall back to a conservative 30 min
		// so a cached-forever token doesn't bite us if the provider drops it.
		tok.ExpiresIn = 1800
	}

	// Refresh proactively at 80% of the TTL.
	c.token = tok.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second * 80 / 100)
	return c.token, nil
}

// Invalidate forces the next Get to refetch. Useful if a downstream API
// returns 401 with a token we thought was valid (e.g. provider revoked it).
func (c *TokenCache) Invalidate() {
	c.mu.Lock()
	c.token = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}
