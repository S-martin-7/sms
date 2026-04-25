package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHorisenCallbackAuth_basic(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mw := HorisenCallbackAuth("test", HorisenCallbackAuthConfig{
		BasicUser: "horisen",
		BasicPass: "s3cret",
	})(next)

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

func TestHorisenCallbackAuth_querySig(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := HorisenCallbackAuth("test", HorisenCallbackAuthConfig{
		QuerySecret: "topsecret",
	})(next)

	cases := []struct {
		name     string
		url      string
		wantCode int
	}{
		{"missing sig", "/x", http.StatusUnauthorized},
		{"empty sig", "/x?sig=", http.StatusUnauthorized},
		{"wrong sig", "/x?sig=nope", http.StatusUnauthorized},
		{"valid sig", "/x?sig=topsecret", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, nil)
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}

func TestHorisenCallbackAuth_eitherModeWorks(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := HorisenCallbackAuth("test", HorisenCallbackAuthConfig{
		BasicUser:   "horisen",
		BasicPass:   "s3cret",
		QuerySecret: "topsecret",
	})(next)

	t.Run("basic only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.SetBasicAuth("horisen", "s3cret")
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("basic only: status = %d, want 200", rec.Code)
		}
	})
	t.Run("sig only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/x?sig=topsecret", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("sig only: status = %d, want 200", rec.Code)
		}
	})
	t.Run("neither", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("neither: status = %d, want 401", rec.Code)
		}
	})
}

func TestHorisenCallbackAuth_failsClosedWhenUnconfigured(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next must not be called when middleware is unconfigured")
	})
	mw := HorisenCallbackAuth("test", HorisenCallbackAuthConfig{})(next)

	req := httptest.NewRequest(http.MethodPost, "/x?sig=anything", nil)
	req.SetBasicAuth("anything", "anything")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when middleware unconfigured", rec.Code)
	}
}
