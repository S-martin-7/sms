package horisen

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client talks to the Horisen SMS HTTP API.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// Config holds the wiring for a Client.
type Config struct {
	BaseURL  string        // e.g. https://sms.horisen.com or https://194.0.137.123:42108
	Username string
	Password string
	Timeout  time.Duration // per-request timeout; defaults to 15s

	// TLSServerName overrides the hostname used for TLS verification.
	// Useful when BaseURL is an IP but the provider's cert is wildcard
	// for a different domain (e.g. *.horisen.pro). Leave empty to use
	// the BaseURL host normally.
	TLSServerName string

	// InsecureSkipVerify disables TLS certificate validation entirely.
	// Only set in non-prod environments. Prefer TLSServerName instead.
	InsecureSkipVerify bool
}

func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("horisen: BaseURL required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("horisen: invalid BaseURL: %w", err)
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("horisen: Username and Password required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSServerName != "" || cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			ServerName:         cfg.TLSServerName,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
	}

	return &Client{
		baseURL:    cfg.BaseURL,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

// SendParams is the minimum payload for a single SMS submission.
type SendParams struct {
	Sender   string            // alphanumeric or E.164
	Receiver string            // E.164 without '+'
	Text     string            // raw text (the client will not detect DCS for you — pass it)
	DCS      DCS               // DCSGSM or DCSUCS
	DLRMask  int               // bitmask, 19 = delivered + undelivered + rejected
	DLRURL   string            // our public DLR callback URL
	Custom   map[string]any    // tenantId, msgId, etc. — Horisen echoes this in DLRs
}

// SendResult is the decoded response from Horisen after a submission.
type SendResult struct {
	Code        Code   `json:"code"`
	Description string `json:"description"`
	MsgID       string `json:"msgId"`
}

type sendRequest struct {
	Type     string         `json:"type"`
	Auth     authBlock      `json:"auth"`
	Sender   string         `json:"sender"`
	Receiver string         `json:"receiver"`
	DCS      DCS            `json:"dcs"`
	Text     string         `json:"text"`
	DLRMask  int            `json:"dlrMask,omitempty"`
	DLRURL   string         `json:"dlrUrl,omitempty"`
	Custom   map[string]any `json:"custom,omitempty"`
}

type authBlock struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sendResponse struct {
	Result SendResult `json:"result"`
}

// errorResponse matches Horisen's error body shape, used when the HTTP
// status is 4xx with a JSON payload like:
//   {"error":{"code":"104","message":"Sending from client's IP not allowed"}}
type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SendSMS posts a single SMS to /bulk/sendsms. Returns the decoded result.
// Returns a *Error when Horisen replies with a non-success code.
// Returns plain errors for transport-level failures (timeout, non-JSON, 5xx).
func (c *Client) SendSMS(ctx context.Context, p SendParams) (*SendResult, error) {
	if p.Sender == "" || p.Receiver == "" || p.Text == "" {
		return nil, errors.New("horisen: sender, receiver and text required")
	}
	if p.DCS == "" {
		p.DCS = DetectDCS(p.Text)
	}

	body, err := json.Marshal(sendRequest{
		Type:     "text",
		Auth:     authBlock{Username: c.username, Password: c.password},
		Sender:   p.Sender,
		Receiver: p.Receiver,
		DCS:      p.DCS,
		Text:     p.Text,
		DLRMask:  p.DLRMask,
		DLRURL:   p.DLRURL,
		Custom:   p.Custom,
	})
	if err != nil {
		return nil, fmt.Errorf("horisen: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bulk/sendsms", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("horisen: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("horisen: do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("horisen: read response: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("horisen: upstream %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusOK {
		// Try to decode Horisen's error body shape. If the code field is
		// populated, surface it as a *Error so the caller can classify
		// retryable vs permanent.
		var errBody errorResponse
		if err := json.Unmarshal(raw, &errBody); err == nil && errBody.Error.Code != "" {
			var n int
			if _, perr := fmt.Sscanf(errBody.Error.Code, "%d", &n); perr == nil {
				return nil, &Error{Code: Code(n), Description: errBody.Error.Message}
			}
		}
		return nil, fmt.Errorf("horisen: http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var parsed sendResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("horisen: decode response: %w (body=%q)", err, truncate(string(raw), 200))
	}

	if !IsSuccess(parsed.Result.Code) {
		return &parsed.Result, &Error{Code: parsed.Result.Code, Description: parsed.Result.Description}
	}
	return &parsed.Result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
