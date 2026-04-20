package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseJWT(t *testing.T) {
	secret := []byte("test-secret-0123456789abcdef")
	tok, exp, err := IssueJWT(secret, JWTClaims{Sub: 42, Role: "superadmin"}, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if time.Until(exp) > time.Hour+time.Minute || time.Until(exp) < 30*time.Minute {
		t.Errorf("unexpected exp: %v", exp)
	}

	claims, err := ParseJWT(secret, tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Sub != 42 {
		t.Errorf("Sub = %d, want 42", claims.Sub)
	}
	if claims.Role != "superadmin" {
		t.Errorf("Role = %q, want superadmin", claims.Role)
	}
}

func TestParseJWT_wrongSecret(t *testing.T) {
	tok, _, _ := IssueJWT([]byte("sec-a"), JWTClaims{Sub: 1, Role: "operator"}, time.Hour)
	if _, err := ParseJWT([]byte("sec-b"), tok); err == nil {
		t.Error("expected error parsing with wrong secret")
	}
}

func TestParseJWT_expired(t *testing.T) {
	secret := []byte("sec")
	tok, _, _ := IssueJWT(secret, JWTClaims{Sub: 1, Role: "operator"}, -time.Minute)
	if _, err := ParseJWT(secret, tok); err == nil {
		t.Error("expected error for expired token")
	}
}
