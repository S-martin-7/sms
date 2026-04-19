package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL     string
	DatabaseURLTest string
	BindAddr        string
	JWTSecret       string
	JWTTTLHours     int
	APIKeyPepper    string
	BcryptCost      int
	LogLevel        string
	Env             string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		DatabaseURLTest: os.Getenv("DATABASE_URL_TEST"),
		BindAddr:        envOr("BIND_ADDR", "127.0.0.1:7300"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		APIKeyPepper:    os.Getenv("API_KEY_PEPPER"),
		LogLevel:        envOr("LOG_LEVEL", "info"),
		Env:             envOr("ENV", "dev"),
	}
	cfg.JWTTTLHours = envInt("JWT_TTL_HOURS", 12)
	cfg.BcryptCost = envInt("BCRYPT_COST", 12)

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.APIKeyPepper == "" {
		missing = append(missing, "API_KEY_PEPPER")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %v", missing)
	}

	if cfg.Env == "prod" {
		if len(cfg.JWTSecret) < 64 {
			return nil, errors.New("JWT_SECRET must be >= 64 chars in prod")
		}
		if len(cfg.APIKeyPepper) < 32 {
			return nil, errors.New("API_KEY_PEPPER must be >= 32 chars in prod")
		}
	}

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
