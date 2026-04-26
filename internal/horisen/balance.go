package horisen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Balance is the parsed shape returned to the public /v1/balance endpoint.
type Balance struct {
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	UpdatedAt time.Time `json:"updated_at"`
	// Source preserved verbatim for debugging (truncated). Not serialised
	// in the public response.
	rawHorisen json.RawMessage `json:"-"`
}

// BalanceClient queries the Horisen finance/balances endpoint using a
// short-lived OAuth2 bearer token from a TokenCache. Wired separately
// from the SMS-send Client because it talks to a different host (the
// finance/accounts cluster) under a different auth scheme.
type BalanceClient struct {
	url        string
	tokens     *TokenCache
	httpClient *http.Client
}

// BalanceClientConfig wires the BalanceClient.
type BalanceClientConfig struct {
	URL        string        // e.g. https://api.horisen.com/finance/sit/v1/balances/biz-partners/customers
	Tokens     *TokenCache
	Timeout    time.Duration // per-request timeout, default 10s
	HTTPClient *http.Client  // override for tests
}

func NewBalanceClient(cfg BalanceClientConfig) (*BalanceClient, error) {
	if cfg.URL == "" {
		return nil, errors.New("balance: URL required")
	}
	if cfg.Tokens == nil {
		return nil, errors.New("balance: TokenCache required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	return &BalanceClient{
		url:        cfg.URL,
		tokens:     cfg.Tokens,
		httpClient: hc,
	}, nil
}

// horisenBalanceResp is the upstream response shape. Horisen's exact
// JSON varies by tenant configuration; we keep the unmarshal forgiving
// (extra keys ignored) and only require the two fields we surface.
type horisenBalanceResp struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	// Some Horisen variants nest the value:
	//   { "balance": { "amount": ..., "currency": ... } }
	// Try the nested shape too.
	Balance *struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"balance,omitempty"`
}

// Get fetches the current balance from Horisen. On HTTP 401 it invalidates
// the token cache and retries once — handles the case where Horisen
// rotated/revoked the token between issuance and use.
func (c *BalanceClient) Get(ctx context.Context) (*Balance, error) {
	bal, err := c.fetch(ctx)
	if errors.Is(err, errBalanceUnauthorized) {
		c.tokens.Invalidate()
		return c.fetch(ctx)
	}
	return bal, err
}

var errBalanceUnauthorized = errors.New("balance: 401 from horisen")

func (c *BalanceClient) fetch(ctx context.Context) (*Balance, error) {
	tok, err := c.tokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("balance: token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("balance: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("balance: do request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errBalanceUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("balance: http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed horisenBalanceResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("balance: decode: %w (body=%q)", err, truncate(string(body), 200))
	}
	out := &Balance{UpdatedAt: time.Now().UTC(), rawHorisen: body}
	if parsed.Balance != nil {
		out.Amount = parsed.Balance.Amount
		out.Currency = parsed.Balance.Currency
	} else {
		out.Amount = parsed.Amount
		out.Currency = parsed.Currency
	}
	if out.Currency == "" {
		return nil, fmt.Errorf("balance: response missing currency (raw=%q)", truncate(string(body), 200))
	}
	return out, nil
}
