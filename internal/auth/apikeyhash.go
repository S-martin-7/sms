package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	tokenPrefix      = "sk_live_"
	tokenRandomLen   = 32
	prefixVisibleLen = 12
	fullTokenLen     = 51 // len("sk_live_") + 43 (base64url no-pad of 32 bytes)
)

// GenerateToken produces a fresh API key and returns (fullToken, prefix, err).
// The full token is what the tenant uses; the prefix is what we persist for lookup.
func GenerateToken() (string, string, error) {
	buf := make([]byte, tokenRandomLen)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("rand read: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	full := tokenPrefix + encoded
	if len(full) != fullTokenLen {
		return "", "", fmt.Errorf("unexpected token len %d", len(full))
	}
	return full, full[:prefixVisibleLen], nil
}

// HashToken returns hex(sha256(token + pepper)).
func HashToken(token, pepper string) string {
	h := sha256.New()
	h.Write([]byte(token))
	h.Write([]byte(pepper))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyToken performs a constant-time compare of the computed hash against
// the stored hash.
func VerifyToken(token, storedHash, pepper string) bool {
	computed := HashToken(token, pepper)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// PrefixOf extracts the persisted prefix from a full token.
// Returns "", false if the token has an unexpected shape.
func PrefixOf(token string) (string, bool) {
	if len(token) != fullTokenLen || token[:len(tokenPrefix)] != tokenPrefix {
		return "", false
	}
	return token[:prefixVisibleLen], true
}
