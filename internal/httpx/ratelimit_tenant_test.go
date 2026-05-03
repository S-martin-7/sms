package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test_TenantSMSLimiter_PerTenantBuckets verifies that hammering the
// limiter from one tenant doesn't affect another tenant's bucket.
func Test_TenantSMSLimiter_PerTenantBuckets(t *testing.T) {
	l := NewTenantSMSLimiter(1, 2) // 1 rps, burst 2

	hits := 0
	h := l.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	call := func(tenantID int64) int {
		req := httptest.NewRequest("POST", "/v1/sms", nil)
		req = req.WithContext(SetTenantID(context.Background(), tenantID))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	// Tenant 1 burns its burst: first 2 are 200, third is 429.
	if got := call(1); got != http.StatusOK {
		t.Fatalf("tenant 1 attempt 1: got %d want 200", got)
	}
	if got := call(1); got != http.StatusOK {
		t.Fatalf("tenant 1 attempt 2: got %d want 200", got)
	}
	if got := call(1); got != http.StatusTooManyRequests {
		t.Fatalf("tenant 1 attempt 3: got %d want 429", got)
	}
	// Tenant 2 has its own bucket — should still pass.
	if got := call(2); got != http.StatusOK {
		t.Fatalf("tenant 2 attempt 1: got %d want 200 (independent bucket)", got)
	}
	if hits != 3 {
		t.Errorf("handler hit %d times, want 3", hits)
	}
}

// Test_TenantSMSLimiter_NoTenantContext verifies that requests without
// a tenant id (shouldn't happen in practice — APIKey middleware injects
// it) are passed through rather than blocked.
func Test_TenantSMSLimiter_NoTenantContext(t *testing.T) {
	l := NewTenantSMSLimiter(0.1, 1) // very tight limit
	called := false
	h := l.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/v1/sms", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !called {
		t.Error("handler not called for request without tenant id")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
