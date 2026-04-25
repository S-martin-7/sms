package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mw := BasicAuth("test", "horisen", "s3cret")(next)

	cases := []struct {
		name     string
		setAuth  bool
		user     string
		pass     string
		wantCode int
	}{
		{"no creds", false, "", "", http.StatusUnauthorized},
		{"wrong user", true, "nope", "s3cret", http.StatusUnauthorized},
		{"wrong pass", true, "horisen", "nope", http.StatusUnauthorized},
		{"valid", true, "horisen", "s3cret", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.setAuth {
				req.SetBasicAuth(tc.user, tc.pass)
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantCode == http.StatusUnauthorized {
				if got := rec.Header().Get("WWW-Authenticate"); got == "" {
					t.Errorf("missing WWW-Authenticate header on 401")
				}
			}
		})
	}
}

func TestBasicAuth_failsClosedWhenUnconfigured(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next must not be called when middleware is unconfigured")
	})
	mw := BasicAuth("test", "", "")(next)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.SetBasicAuth("anything", "anything")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when middleware unconfigured", rec.Code)
	}
}
