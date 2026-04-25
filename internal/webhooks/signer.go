// Package webhooks implements signed outbound HTTP delivery of events to
// tenant-supplied URLs. The signature scheme follows Stripe's: a header
// X-Signature: t=<unix>,v1=<hex(hmac_sha256(secret, t.body))>. Tenants
// recompute the same value to verify authenticity and rebuild the same
// JSON they POSTed to the API.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HeaderSignature is the HTTP header that carries the signature.
const HeaderSignature = "X-Signature"

// SignatureSchemeV1 is the only currently supported signature version.
const SignatureSchemeV1 = "v1"

// MaxClockSkew is the tolerance window (each side) for verification.
// Tenants outside this window should reject the request as a replay.
const MaxClockSkew = 5 * time.Minute

// Sign builds the X-Signature header value for the given body and secret
// at time `at`. The output format is `t=<unix>,v1=<hex>`.
func Sign(secret string, body []byte, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,%s=%s", ts, SignatureSchemeV1, sig)
}

// Verify checks the X-Signature header against `body` using `secret`. It
// returns nil on a valid signature within tolerance, or an error otherwise.
//
// Exposed mostly for tests and tenant SDKs — the dispatcher itself only
// signs (it never verifies its own signatures).
func Verify(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	ts, sig, err := parseHeader(header)
	if err != nil {
		return err
	}
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return fmt.Errorf("webhooks: signature timestamp outside tolerance (%s)", d)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	want := mac.Sum(nil)
	got, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("webhooks: signature hex decode: %w", err)
	}
	if !hmac.Equal(want, got) {
		return errors.New("webhooks: signature mismatch")
	}
	return nil
}

// parseHeader pulls the unix timestamp and v1 signature out of the
// X-Signature value. Unknown comma-separated parts are ignored so newer
// schemes can coexist with v1 during migration.
func parseHeader(h string) (ts int64, sig string, err error) {
	for _, part := range strings.Split(h, ",") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, perr := strconv.ParseInt(v, 10, 64)
			if perr != nil {
				return 0, "", fmt.Errorf("webhooks: invalid t=: %w", perr)
			}
			ts = n
		case SignatureSchemeV1:
			sig = v
		}
	}
	if ts == 0 {
		return 0, "", errors.New("webhooks: missing t= in signature header")
	}
	if sig == "" {
		return 0, "", errors.New("webhooks: missing v1= in signature header")
	}
	return ts, sig, nil
}
