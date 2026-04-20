package horisen

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNew_rejectsMissingConfig(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("expected error for empty config")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Error("expected error for missing creds")
	}
}

func TestSendSMS_happyPath(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/bulk/sendsms" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		var req sendRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("bad body: %v", err)
		}
		if req.Auth.Username != "user" || req.Auth.Password != "pass" {
			t.Errorf("auth not forwarded: %+v", req.Auth)
		}
		if req.Sender != "Test" || req.Receiver != "4179000000" || req.Text != "hola" {
			t.Errorf("payload: %+v", req)
		}
		if req.DCS != DCSGSM {
			t.Errorf("dcs = %v, want GSM", req.DCS)
		}
		if req.Custom["msgId"] != "m-1" {
			t.Errorf("custom not forwarded: %v", req.Custom)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"code":100,"description":"OK","msgId":"abc-123"}}`))
	})

	res, err := c.SendSMS(context.Background(), SendParams{
		Sender:   "Test",
		Receiver: "4179000000",
		Text:     "hola",
		DLRMask:  19,
		DLRURL:   "https://example.com/dlr",
		Custom:   map[string]any{"msgId": "m-1", "tenantId": 7},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Code != 100 || res.MsgID != "abc-123" {
		t.Errorf("result = %+v", res)
	}
}

func TestSendSMS_autoDetectsDCS(t *testing.T) {
	var captured DCS
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req sendRequest
		_ = json.Unmarshal(body, &req)
		captured = req.DCS
		_, _ = w.Write([]byte(`{"result":{"code":100,"description":"OK","msgId":"x"}}`))
	})
	_, err := c.SendSMS(context.Background(), SendParams{
		Sender: "S", Receiver: "4179000000", Text: "Hi 👋 emoji",
		// DCS left empty on purpose
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured != DCSUCS {
		t.Errorf("auto-detected DCS = %v, want UCS", captured)
	}
}

func TestSendSMS_horisenError_returnsTypedError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"code":103,"description":"invalid receiver","msgId":""}}`))
	})
	_, err := c.SendSMS(context.Background(), SendParams{
		Sender: "S", Receiver: "9", Text: "x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("err is not *horisen.Error: %T %v", err, err)
	}
	if herr.Code != 103 || herr.Description != "invalid receiver" {
		t.Errorf("err = %+v", herr)
	}
	if IsRetryable(herr.Code) {
		t.Error("103 should not be retryable")
	}
}

func TestSendSMS_throttled_isRetryable(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"code":105,"description":"throttled","msgId":""}}`))
	})
	_, err := c.SendSMS(context.Background(), SendParams{
		Sender: "S", Receiver: "4179000000", Text: "x",
	})
	var herr *Error
	if !errors.As(err, &herr) || !IsRetryable(herr.Code) {
		t.Fatalf("want retryable *Error, got %T %v", err, err)
	}
}

func TestSendSMS_http420_withHorisenErrorBody_returnsTypedError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(420)
		_, _ = w.Write([]byte(`{"error":{"code":"104","message":"Sending from client's IP not allowed"}}`))
	})
	_, err := c.SendSMS(context.Background(), SendParams{
		Sender: "S", Receiver: "4179000000", Text: "x",
	})
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if herr.Code != 104 {
		t.Errorf("code = %d, want 104", herr.Code)
	}
	if IsRetryable(herr.Code) {
		t.Error("104 must not be retryable")
	}
}

func TestSendSMS_http5xx_isTransportError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.SendSMS(context.Background(), SendParams{
		Sender: "S", Receiver: "4179000000", Text: "x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upstream 500") {
		t.Errorf("err = %v, want upstream 500", err)
	}
	var herr *Error
	if errors.As(err, &herr) {
		t.Errorf("5xx should NOT be a *horisen.Error, got %+v", herr)
	}
}

func TestSendSMS_missingFields(t *testing.T) {
	c, _ := New(Config{BaseURL: "http://x", Username: "u", Password: "p"})
	_, err := c.SendSMS(context.Background(), SendParams{})
	if err == nil {
		t.Error("expected error for empty params")
	}
}
