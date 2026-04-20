package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	Sub  int64
	Role string
}

type internalClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// IssueJWT issues an HS256 token signed with secret. Returns (token, expiry).
func IssueJWT(secret []byte, c JWTClaims, ttl time.Duration) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)
	claims := internalClaims{
		Role: c.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", c.Sub),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	return s, exp, nil
}

// ParseJWT verifies signature + expiry and returns the claims.
func ParseJWT(secret []byte, tokenStr string) (JWTClaims, error) {
	var ic internalClaims
	_, err := jwt.ParseWithClaims(tokenStr, &ic, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected alg %v", t.Method)
		}
		return secret, nil
	})
	if err != nil {
		return JWTClaims{}, err
	}
	var sub int64
	if _, err := fmt.Sscanf(ic.Subject, "%d", &sub); err != nil {
		return JWTClaims{}, fmt.Errorf("parse sub: %w", err)
	}
	return JWTClaims{Sub: sub, Role: ic.Role}, nil
}
