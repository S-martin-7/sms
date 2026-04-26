package horisen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenCache_fetchesAndCaches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.FormValue("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	cache := NewTokenCache(OAuthConfig{
		ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL,
		HTTPClient: srv.Client(),
	})

	for i := 0; i < 3; i++ {
		got, err := cache.Get(context.Background())
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		if got != "tok-123" {
			t.Errorf("token = %q", got)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 fetch, got %d", hits.Load())
	}
}

func TestTokenCache_refreshesAfterExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"expires_in":   1, // 1s, so 80% = 800ms cache
		})
	}))
	defer srv.Close()

	cache := NewTokenCache(OAuthConfig{
		ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL,
		HTTPClient: srv.Client(),
	})

	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(900 * time.Millisecond)
	if _, err := cache.Get(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if hits.Load() != 2 {
		t.Errorf("expected 2 fetches after expiry, got %d", hits.Load())
	}
}

func TestTokenCache_invalidateForcesRefetch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok", "expires_in": 3600,
		})
	}))
	defer srv.Close()
	cache := NewTokenCache(OAuthConfig{
		ClientID: "id", ClientSecret: "secret", TokenURL: srv.URL,
		HTTPClient: srv.Client(),
	})
	_, _ = cache.Get(context.Background())
	cache.Invalidate()
	_, _ = cache.Get(context.Background())
	if hits.Load() != 2 {
		t.Errorf("invalidate should force refetch, got %d hits", hits.Load())
	}
}

func TestTokenCache_propagatesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	cache := NewTokenCache(OAuthConfig{
		ClientID: "bad", ClientSecret: "bad", TokenURL: srv.URL,
		HTTPClient: srv.Client(),
	})
	if _, err := cache.Get(context.Background()); err == nil {
		t.Error("expected error on 401")
	}
}

func TestTokenCache_rejectsMissingConfig(t *testing.T) {
	cache := NewTokenCache(OAuthConfig{})
	if _, err := cache.Get(context.Background()); err == nil {
		t.Error("expected error when config is empty")
	}
}
