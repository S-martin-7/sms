package config

import "testing"

func TestLoad_missingRequiredFails(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("API_KEY_PEPPER", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when required vars missing, got nil")
	}
}

func TestLoad_happyPath(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("API_KEY_PEPPER", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BIND_ADDR", "127.0.0.1:7300")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("ENV", "dev")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BindAddr != "127.0.0.1:7300" {
		t.Errorf("BindAddr = %q, want %q", cfg.BindAddr, "127.0.0.1:7300")
	}
	if cfg.JWTTTLHours != 12 {
		t.Errorf("JWTTTLHours = %d, want 12 (default)", cfg.JWTTTLHours)
	}
	if cfg.BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want 12 (default)", cfg.BcryptCost)
	}
}

func TestLoad_prodRejectsShortSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "tooShort")
	t.Setenv("API_KEY_PEPPER", "tooShort")
	t.Setenv("ENV", "prod")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short prod secrets, got nil")
	}
}
