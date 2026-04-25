package httpx

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cursor format note: every cursor we emit starts with a version tag so we
// can rotate the encoding (add fields, change ordering) without breaking
// clients that still hold pre-rotation cursors. Version mismatch → invalid.
const cursorVersion = "v1"

// ErrInvalidCursor is returned when a cursor cannot be decoded. Handlers
// should map this to HTTP 400.
var ErrInvalidCursor = errors.New("invalid cursor")

// EncodeIntCursor encodes a single int64 (e.g. events.id) as an opaque
// base64url string with a version prefix.
func EncodeIntCursor(id int64) string {
	return encodeCursor(strconv.FormatInt(id, 10))
}

// DecodeIntCursor reverses EncodeIntCursor. An empty string returns 0
// (callers treat 0 as "from newest").
func DecodeIntCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	body, err := decodeCursor(s)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(body, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: not an int", ErrInvalidCursor)
	}
	if id <= 0 {
		return 0, fmt.Errorf("%w: must be positive", ErrInvalidCursor)
	}
	return id, nil
}

// EncodeMsgCursor encodes (created_at, id) for paginating tables ordered by
// (created_at DESC, id DESC) where the PK is a non-monotonic UUID. We pack
// unix nanos plus the UUID string so the decoder gets exactly what the SQL
// tuple-comparison needs.
func EncodeMsgCursor(createdAt time.Time, id uuid.UUID) string {
	return encodeCursor(fmt.Sprintf("%d:%s", createdAt.UTC().UnixNano(), id.String()))
}

// DecodeMsgCursor reverses EncodeMsgCursor. Empty string returns zero time
// + nil UUID (caller treats that as "from newest").
func DecodeMsgCursor(s string) (time.Time, uuid.UUID, error) {
	if s == "" {
		return time.Time{}, uuid.Nil, nil
	}
	body, err := decodeCursor(s)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(body, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: missing : separator", ErrInvalidCursor)
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: bad nanos", ErrInvalidCursor)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: bad uuid", ErrInvalidCursor)
	}
	return time.Unix(0, nanos).UTC(), id, nil
}

// encodeCursor wraps a payload with the version prefix and base64url-encodes
// the result. base64url (no padding) keeps cursors safe to drop into URL
// query params without escaping.
func encodeCursor(payload string) string {
	raw := cursorVersion + ":" + payload
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("%w: not base64url", ErrInvalidCursor)
	}
	prefix := cursorVersion + ":"
	if !strings.HasPrefix(string(raw), prefix) {
		return "", fmt.Errorf("%w: unknown version", ErrInvalidCursor)
	}
	return strings.TrimPrefix(string(raw), prefix), nil
}
