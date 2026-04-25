package httpx

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntCursor_roundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, 12345, 9223372036854775807} {
		enc := EncodeIntCursor(id)
		got, err := DecodeIntCursor(enc)
		if err != nil {
			t.Errorf("decode %d: %v", id, err)
			continue
		}
		if got != id {
			t.Errorf("round trip mismatch: %d → %d", id, got)
		}
	}
}

func TestIntCursor_emptyMeansZero(t *testing.T) {
	got, err := DecodeIntCursor("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != 0 {
		t.Errorf("empty → %d, want 0", got)
	}
}

func TestIntCursor_invalid(t *testing.T) {
	cases := map[string]string{
		"not base64":     "??not-base64??",
		"wrong version":  encodeCursor("99"), // looks fine but DecodeIntCursor expects v1; encodeCursor uses v1 so this passes by design — make a manual one
		"manual v2":      "djI6MTIz",         // base64url("v2:123")
		"missing colon":  "djE",              // base64url("v1") — no colon
		"non-int body":   encodeCursor("notint"),
		"negative":       encodeCursor("-5"),
		"zero":           encodeCursor("0"),
	}
	// Remove the "wrong version" case since encodeCursor() always uses v1.
	delete(cases, "wrong version")
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeIntCursor(in)
			if err == nil {
				t.Errorf("expected error for %q", in)
				return
			}
			if !errors.Is(err, ErrInvalidCursor) {
				t.Errorf("err = %v, want wrapping ErrInvalidCursor", err)
			}
		})
	}
}

func TestMsgCursor_roundTrip(t *testing.T) {
	now := time.Date(2026, 4, 25, 13, 17, 30, 12345678, time.UTC)
	id := uuid.MustParse("ceb644bb-6dc4-4048-ba29-8a8eddbbf747")

	enc := EncodeMsgCursor(now, id)
	gotTime, gotID, err := DecodeMsgCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotTime.Equal(now) {
		t.Errorf("time mismatch: got %v, want %v", gotTime, now)
	}
	if gotID != id {
		t.Errorf("id mismatch: got %s, want %s", gotID, id)
	}
}

func TestMsgCursor_emptyMeansZero(t *testing.T) {
	gotTime, gotID, err := DecodeMsgCursor("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !gotTime.IsZero() {
		t.Errorf("empty time should be zero, got %v", gotTime)
	}
	if gotID != uuid.Nil {
		t.Errorf("empty uuid should be nil, got %s", gotID)
	}
}

func TestMsgCursor_invalid(t *testing.T) {
	cases := map[string]string{
		"not base64":      "??",
		"missing colon":   encodeCursor("abc"),
		"bad nanos":       encodeCursor("notnum:" + uuid.New().String()),
		"bad uuid":        encodeCursor("123:not-a-uuid"),
		"v2 prefix":       "djI6MTIzOmFiYw", // base64url("v2:123:abc")
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := DecodeMsgCursor(in)
			if err == nil {
				t.Errorf("expected error for %q", in)
				return
			}
			if !errors.Is(err, ErrInvalidCursor) {
				t.Errorf("err = %v, want wrapping ErrInvalidCursor", err)
			}
		})
	}
}

func TestEncodedCursorIsURLSafe(t *testing.T) {
	// base64url uses [A-Za-z0-9-_], no '+' or '/', no padding '='.
	// Pick something likely to produce padding chars in standard base64.
	enc := EncodeMsgCursor(time.Unix(1, 234567890), uuid.New())
	if strings.ContainsAny(enc, "+/=") {
		t.Errorf("cursor has unsafe chars for URL: %s", enc)
	}
}
