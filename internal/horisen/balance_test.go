package horisen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// makeOAuthSrv stands up a fake token endpoint that always returns the
// given token. Used so balance tests don't have to reimplement the OAuth
// dance per case.
func makeOAuthSrv(t *testing.T, token string) (*httptest.Server, *TokenCache) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": token,
			"expires_in":   3600,
		})
	}))
	t.Cleanup(srv.Close)
	cache := NewTokenCache(OAuthConfig{
		ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL,
		HTTPClient: srv.Client(),
	})
	return srv, cache
}

func TestBalance_happyPath(t *testing.T) {
	_, tokens := makeOAuthSrv(t, "tok-1")

	balSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-1" {
			t.Errorf("auth header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"amount":   123.45,
			"currency": "USD",
		})
	}))
	defer balSrv.Close()

	bc, err := NewBalanceClient(BalanceClientConfig{
		URL: balSrv.URL, Tokens: tokens, HTTPClient: balSrv.Client(),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, err := bc.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Amount != 123.45 || got.Currency != "USD" {
		t.Errorf("balance = %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestBalance_acceptsNestedShape(t *testing.T) {
	_, tokens := makeOAuthSrv(t, "tok-1")
	balSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"balance": map[string]any{"amount": 50.0, "currency": "EUR"},
		})
	}))
	defer balSrv.Close()
	bc, _ := NewBalanceClient(BalanceClientConfig{
		URL: balSrv.URL, Tokens: tokens, HTTPClient: balSrv.Client(),
	})
	got, err := bc.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Amount != 50.0 || got.Currency != "EUR" {
		t.Errorf("nested balance = %+v", got)
	}
}

func TestBalance_retriesOn401WithFreshToken(t *testing.T) {
	// Token endpoint returns different tokens on each call.
	var tokenCalls atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("tok-%d", n),
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	tokens := NewTokenCache(OAuthConfig{
		ClientID: "id", ClientSecret: "secret", TokenURL: tokenSrv.URL,
		HTTPClient: tokenSrv.Client(),
	})

	// Balance returns 401 the first time, 200 with tok-2.
	var balCalls atomic.Int32
	balSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := balCalls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-2" {
			t.Errorf("retry auth = %q, want Bearer tok-2", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"amount": 10.0, "currency": "USD",
		})
	}))
	defer balSrv.Close()

	bc, _ := NewBalanceClient(BalanceClientConfig{
		URL: balSrv.URL, Tokens: tokens, HTTPClient: balSrv.Client(),
	})
	got, err := bc.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Amount != 10.0 {
		t.Errorf("amount = %v", got.Amount)
	}
	if balCalls.Load() != 2 {
		t.Errorf("expected 2 balance calls (401 + retry), got %d", balCalls.Load())
	}
	if tokenCalls.Load() != 2 {
		t.Errorf("expected 2 token fetches (initial + invalidate), got %d", tokenCalls.Load())
	}
}

func TestBalance_propagates5xx(t *testing.T) {
	_, tokens := makeOAuthSrv(t, "tok")
	balSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer balSrv.Close()
	bc, _ := NewBalanceClient(BalanceClientConfig{
		URL: balSrv.URL, Tokens: tokens, HTTPClient: balSrv.Client(),
	})
	if _, err := bc.Get(context.Background()); err == nil {
		t.Error("expected error on 500")
	}
}

func TestBalance_rejectsMissingCurrency(t *testing.T) {
	_, tokens := makeOAuthSrv(t, "tok")
	balSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"amount": 5.0})
	}))
	defer balSrv.Close()
	bc, _ := NewBalanceClient(BalanceClientConfig{
		URL: balSrv.URL, Tokens: tokens, HTTPClient: balSrv.Client(),
	})
	if _, err := bc.Get(context.Background()); err == nil {
		t.Error("expected error when currency missing")
	}
}
