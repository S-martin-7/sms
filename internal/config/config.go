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

	// Horisen provider config (required once we enable /v1/sms send).
	HorisenBaseURL         string
	HorisenUsername        string
	HorisenPassword        string
	HorisenTPS             int
	HorisenValiditySec     int
	HorisenCallbackSecret  string
	HorisenCallbackUser    string
	HorisenCallbackPass    string
	HorisenTLSServerName   string
	PublicBaseURL          string

	// OAuth2 client_credentials wiring for the Balance API and any other
	// finance/customer endpoints that require a bearer token instead of
	// the SMS-send Basic auth.
	HorisenOAuthClientID     string
	HorisenOAuthClientSecret string
	HorisenOAuthTokenURL     string
	HorisenBalanceURL        string
	BalanceCacheSeconds      int

	// Per-tenant rate limit on POST /v1/sms* (token-bucket). Default
	// 5 req/s / burst 10 is half the global Horisen TPS, leaving room
	// for at least one other tenant to make progress.
	SMSPerTenantTPS   float64
	SMSPerTenantBurst int
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

	cfg.HorisenBaseURL = os.Getenv("HORISEN_BASE_URL")
	cfg.HorisenUsername = os.Getenv("HORISEN_USERNAME")
	cfg.HorisenPassword = os.Getenv("HORISEN_PASSWORD")
	cfg.HorisenTPS = envInt("HORISEN_TPS", 10)
	cfg.HorisenValiditySec = envInt("HORISEN_VALIDITY_SEC", 86400)
	cfg.HorisenCallbackSecret = os.Getenv("HORISEN_CALLBACK_SECRET")
	cfg.HorisenCallbackUser = os.Getenv("HORISEN_CALLBACK_USER")
	cfg.HorisenCallbackPass = os.Getenv("HORISEN_CALLBACK_PASSWORD")
	cfg.HorisenTLSServerName = os.Getenv("HORISEN_TLS_SERVER_NAME")
	cfg.PublicBaseURL = os.Getenv("PUBLIC_BASE_URL")
	cfg.HorisenOAuthClientID = os.Getenv("HORISEN_OAUTH_CLIENT_ID")
	cfg.HorisenOAuthClientSecret = os.Getenv("HORISEN_OAUTH_CLIENT_SECRET")
	cfg.HorisenOAuthTokenURL = os.Getenv("HORISEN_OAUTH_TOKEN_URL")
	cfg.HorisenBalanceURL = os.Getenv("HORISEN_BALANCE_URL")
	cfg.BalanceCacheSeconds = envInt("BALANCE_CACHE_SECONDS", 60)
	cfg.SMSPerTenantTPS = envFloat("SMS_PER_TENANT_TPS", 5)
	cfg.SMSPerTenantBurst = envInt("SMS_PER_TENANT_BURST", 10)

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

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
