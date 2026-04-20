package auth

import (
	"strings"
	"testing"
)

const testPepper = "test-pepper-0123456789abcdef"

func TestGenerateToken_shape(t *testing.T) {
	tok, prefix, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !strings.HasPrefix(tok, "sk_live_") {
		t.Errorf("token should start with sk_live_, got %q", tok)
	}
	if len(tok) != 51 {
		t.Errorf("token len = %d, want 51", len(tok))
	}
	if len(prefix) != 12 {
		t.Errorf("prefix len = %d, want 12", len(prefix))
	}
	if !strings.HasPrefix(tok, prefix) {
		t.Errorf("token should start with prefix %q", prefix)
	}
}

func TestGenerateToken_unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		tok, _, err := GenerateToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatalf("collision at iter %d", i)
		}
		seen[tok] = true
	}
}

func TestHashAndVerify(t *testing.T) {
	tok := "sk_live_"+"abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ"
	h := HashToken(tok, testPepper)
	if len(h) != 64 {
		t.Errorf("hash len = %d, want 64 (hex sha256)", len(h))
	}
	if !VerifyToken(tok, h, testPepper) {
		t.Error("verify should return true for matching token")
	}
	if VerifyToken("sk_live_wrong", h, testPepper) {
		t.Error("verify should return false for non-matching token")
	}
}

func TestHashToken_peppered(t *testing.T) {
	tok := "sk_live_example"
	h1 := HashToken(tok, "pepperA")
	h2 := HashToken(tok, "pepperB")
	if h1 == h2 {
		t.Error("different peppers should produce different hashes")
	}
}
