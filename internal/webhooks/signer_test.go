package webhooks

import (
	"strings"
	"testing"
	"time"
)

func TestSign_format(t *testing.T) {
	at := time.Unix(1700000000, 0)
	got := Sign("topsecret", []byte(`{"event":"sms.delivered"}`), at)
	if !strings.HasPrefix(got, "t=1700000000,v1=") {
		t.Errorf("unexpected format: %s", got)
	}
}

func TestSign_isStable(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`{"hello":"world"}`)
	a := Sign("k", body, at)
	b := Sign("k", body, at)
	if a != b {
		t.Errorf("Sign not deterministic: %s vs %s", a, b)
	}
}

func TestSign_secretMatters(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`x`)
	a := Sign("k1", body, at)
	b := Sign("k2", body, at)
	if a == b {
		t.Error("expected different signatures for different secrets")
	}
}

func TestVerify_roundTrip(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`{"event":"sms.delivered","msg_id":"abc"}`)
	header := Sign("topsecret", body, at)
	if err := Verify("topsecret", header, body, at, MaxClockSkew); err != nil {
		t.Errorf("verify failed: %v", err)
	}
}

func TestVerify_wrongSecret(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`x`)
	header := Sign("good", body, at)
	if err := Verify("bad", header, body, at, MaxClockSkew); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestVerify_modifiedBody(t *testing.T) {
	at := time.Unix(1700000000, 0)
	header := Sign("k", []byte(`original`), at)
	if err := Verify("k", header, []byte(`tampered`), at, MaxClockSkew); err == nil {
		t.Error("expected mismatch on body tamper")
	}
}

func TestVerify_replayOutsideTolerance(t *testing.T) {
	signedAt := time.Unix(1700000000, 0)
	body := []byte(`x`)
	header := Sign("k", body, signedAt)

	now := signedAt.Add(10 * time.Minute) // older than 5m tolerance
	if err := Verify("k", header, body, now, MaxClockSkew); err == nil {
		t.Error("expected timestamp-outside-tolerance error")
	}
}

func TestVerify_malformedHeader(t *testing.T) {
	cases := []string{
		"",
		"not-key-value",
		"v1=abc",                     // no timestamp
		"t=notanumber,v1=abc",
		"t=1700000000",               // no v1
	}
	for _, h := range cases {
		if err := Verify("k", h, []byte(`x`), time.Unix(1700000000, 0), MaxClockSkew); err == nil {
			t.Errorf("expected error for header %q", h)
		}
	}
}

func TestVerify_ignoresUnknownSchemes(t *testing.T) {
	at := time.Unix(1700000000, 0)
	body := []byte(`x`)
	base := Sign("k", body, at)
	// Add a hypothetical future scheme; v1 must still validate.
	hybrid := base + ",v9=future-signature-that-we-dont-know"
	if err := Verify("k", hybrid, body, at, MaxClockSkew); err != nil {
		t.Errorf("verify failed when unknown scheme present: %v", err)
	}
}
