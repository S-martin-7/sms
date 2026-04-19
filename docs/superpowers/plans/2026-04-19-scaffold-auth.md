# Scaffold + Auth Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational layer of the SMS gateway (repo structure, Postgres setup, auth tables, API-key/JWT verification, minimal HTTP server, and `smsctl` CLI) so the auth pipeline can be exercised end-to-end with curl.

**Architecture:** Go 1.24 module with layered `internal/` packages. Postgres via pgxpool + sqlc for typed queries. API keys use SHA-256+pepper (random 32-byte tokens), admin passwords use bcrypt cost 12. HTTP via chi router. CLI `smsctl` reuses the same domain packages. Integration tests hit a real `sms_test` Postgres DB shared with dev via docker-compose.

**Tech Stack:** Go 1.24, PostgreSQL 15, `github.com/jackc/pgx/v5`, `github.com/go-chi/chi/v5`, `github.com/golang-jwt/jwt/v5`, `github.com/golang-migrate/migrate/v4`, `github.com/rs/zerolog`, `golang.org/x/crypto/bcrypt`, `sqlc` (dev tool).

**Reference spec:** `docs/superpowers/specs/2026-04-19-scaffold-auth-design.md`.

---

## Phase 1 — Repo bootstrap

### Task 1: Initialize Go module and repo metadata

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Modify: `README.md` (minor — note "build underway")

- [ ] **Step 1: Initialize Go module**

Run: `cd C:\codigo\vps\sms && go mod init github.com/S-martin-7/sms`
Expected: creates `go.mod` with `module github.com/S-martin-7/sms` and `go 1.24`.

- [ ] **Step 2: Create `.gitignore`**

```gitignore
# binaries
/bin/
/cmd/server/server
/cmd/smsctl/smsctl
*.exe

# build / test artifacts
/tmp/
*.out
coverage.*

# env / secrets
.env
.env.local

# OS / editor
.DS_Store
.vscode/
.idea/

# postgres volume
/postgres-data/
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: init go module and gitignore"
```

---

### Task 2: Add docker-compose for Postgres (dev + test DBs)

**Files:**
- Create: `docker-compose.yml`
- Create: `deploy/postgres-init.sql`

- [ ] **Step 1: Create `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:15-alpine
    container_name: sms-postgres
    environment:
      POSTGRES_USER: sms
      POSTGRES_PASSWORD: sms
      POSTGRES_DB: sms
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./deploy/postgres-init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sms -d sms"]
      interval: 2s
      timeout: 3s
      retries: 10

volumes:
  postgres-data:
```

- [ ] **Step 2: Create `deploy/postgres-init.sql`**

```sql
-- Runs once when the Postgres volume is first initialized.
-- Creates the test database alongside the main one.

CREATE DATABASE sms_test;
GRANT ALL PRIVILEGES ON DATABASE sms_test TO sms;
```

- [ ] **Step 3: Bring Postgres up and verify**

Run:
```bash
docker compose up -d postgres
docker compose ps
```
Expected: `sms-postgres` status `healthy` within ~10s.

Verify both DBs exist:
```bash
docker exec sms-postgres psql -U sms -l
```
Expected: output lists both `sms` and `sms_test` among the databases.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml deploy/postgres-init.sql
git commit -m "chore: add docker-compose for postgres dev+test"
```

---

### Task 3: Add Makefile with dev targets

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create `Makefile`**

```makefile
.PHONY: up down psql migrate migrate-test sqlc test run build tidy

DATABASE_URL ?= postgres://sms:sms@localhost:5432/sms?sslmode=disable
DATABASE_URL_TEST ?= postgres://sms:sms@localhost:5432/sms_test?sslmode=disable

up:
	docker compose up -d postgres

down:
	docker compose down

psql:
	docker exec -it sms-postgres psql -U sms -d sms

migrate:
	go run ./cmd/smsctl migrate up

migrate-test:
	DATABASE_URL="$(DATABASE_URL_TEST)" go run ./cmd/smsctl migrate up

sqlc:
	sqlc generate

test: migrate-test
	DATABASE_URL_TEST="$(DATABASE_URL_TEST)" go test ./... -count=1

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server
	go build -o bin/smsctl ./cmd/smsctl

tidy:
	go mod tidy
```

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with dev targets"
```

---

## Phase 2 — Config & Logger

### Task 4: Implement `internal/config` with fail-fast env loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"testing"
)

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
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/config/... -run TestLoad -v`
Expected: FAIL (`Load` undefined).

- [ ] **Step 3: Implement `Load()`**

```go
// internal/config/config.go
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
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add fail-fast env loader"
```

---

### Task 5: Implement `internal/logger` with zerolog

**Files:**
- Create: `internal/logger/logger.go`

- [ ] **Step 1: Install zerolog**

Run: `go get github.com/rs/zerolog@latest`

- [ ] **Step 2: Create `logger.go`**

```go
// internal/logger/logger.go
package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type ctxKey struct{}

// New returns a zerolog.Logger honoring the given level string.
// In dev env it uses a human-friendly console writer; in prod, JSON.
func New(level, env string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var w io.Writer = os.Stdout
	if env != "prod" {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}
	return zerolog.New(w).Level(lvl).With().Timestamp().Logger()
}

// FromContext returns the logger stored in ctx, or a disabled one.
func FromContext(ctx context.Context) *zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zerolog.Logger); ok {
		return l
	}
	d := zerolog.Nop()
	return &d
}

// WithLogger returns a ctx carrying l.
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./internal/logger/...`
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/logger/
git commit -m "feat(logger): add zerolog wrapper with ctx helpers"
```

---

## Phase 3 — Database plumbing

### Task 6: Implement `internal/db/pool` for pgxpool

**Files:**
- Create: `internal/db/pool.go`

- [ ] **Step 1: Install pgx**

Run: `go get github.com/jackc/pgx/v5@latest`

- [ ] **Step 2: Create `pool.go`**

```go
// internal/db/pool.go
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open parses dsn, configures sane defaults, and verifies connectivity with a ping.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./internal/db/...`
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/db/pool.go
git commit -m "feat(db): add pgxpool Open helper"
```

---

### Task 7: Create migration `001_auth` up/down SQL

**Files:**
- Create: `internal/db/migrations/001_auth.up.sql`
- Create: `internal/db/migrations/001_auth.down.sql`

- [ ] **Step 1: Create `001_auth.up.sql`**

```sql
CREATE TABLE tenants (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',
  daily_sms_limit INT,
  monthly_budget  NUMERIC(12,4),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  prefix          TEXT NOT NULL UNIQUE,
  hash            TEXT NOT NULL,
  hash_algo       TEXT NOT NULL DEFAULT 'sha256-v1',
  name            TEXT,
  scopes          TEXT[] NOT NULL DEFAULT ARRAY['send','read','webhooks']::TEXT[],
  last_used_at    TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_keys_tenant_active ON api_keys(tenant_id) WHERE revoked_at IS NULL;

CREATE TABLE admin_users (
  id              BIGSERIAL PRIMARY KEY,
  email           TEXT UNIQUE NOT NULL,
  password_hash   TEXT NOT NULL,
  role            TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id              BIGSERIAL PRIMARY KEY,
  actor_id        BIGINT REFERENCES admin_users(id),
  action          TEXT NOT NULL,
  target_type     TEXT,
  target_id       TEXT,
  metadata        JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_actor ON audit_log(actor_id, created_at DESC);
```

- [ ] **Step 2: Create `001_auth.down.sql`**

```sql
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS admin_users;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS tenants;
```

- [ ] **Step 3: Smoke-test by applying manually**

Run (requires compose up):
```bash
docker exec -i sms-postgres psql -U sms -d sms_test < internal/db/migrations/001_auth.up.sql
docker exec sms-postgres psql -U sms -d sms_test -c "\dt"
```
Expected: lists `tenants`, `api_keys`, `admin_users`, `audit_log`.

Rollback:
```bash
docker exec -i sms-postgres psql -U sms -d sms_test < internal/db/migrations/001_auth.down.sql
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/
git commit -m "feat(db): add 001_auth migration"
```

---

### Task 8: Implement migration runner using golang-migrate

**Files:**
- Create: `internal/db/migrate.go`

- [ ] **Step 1: Install golang-migrate**

Run:
```bash
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/golang-migrate/migrate/v4/database/pgx/v5@latest
go get github.com/golang-migrate/migrate/v4/source/iofs@latest
```

- [ ] **Step 2: Create `migrate.go` with embedded migrations**

```go
// internal/db/migrate.go
package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies pending migrations (dir="up") or reverts one (dir="down").
// dir can also be "version" — prints nothing, caller uses Version.
func Migrate(dsn, dir string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxDSN(dsn))
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer m.Close()

	switch dir {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	default:
		return fmt.Errorf("unknown direction %q", dir)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate %s: %w", dir, err)
	}
	return nil
}

// Version returns current schema version. Returns 0 when no migrations applied.
func Version(dsn string) (uint, bool, error) {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return 0, false, err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxDSN(dsn))
	if err != nil {
		return 0, false, err
	}
	defer m.Close()
	v, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, dirty, err
}

// pgxDSN converts a plain postgres DSN to the pgx/v5 driver URL
// expected by golang-migrate (prefix "pgx5://").
func pgxDSN(dsn string) string {
	// Accept both forms. golang-migrate/database/pgx/v5 registers under "pgx5".
	if len(dsn) >= 11 && dsn[:11] == "postgres://" {
		return "pgx5://" + dsn[11:]
	}
	return dsn
}
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./internal/db/...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/db/migrate.go
git commit -m "feat(db): add golang-migrate runner with embedded sql"
```

---

### Task 9: Implement `internal/db/testutil` for integration tests

**Files:**
- Create: `internal/db/testutil.go`

- [ ] **Step 1: Create `testutil.go`**

```go
// internal/db/testutil.go
package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WithTestDB opens a pool against DATABASE_URL_TEST, truncates all auth
// tables, and returns the pool. The test is skipped if the env var
// is unset. Caller should NOT close the pool — the cleanup fn does it.
func WithTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(ctx, `TRUNCATE audit_log, admin_users, api_keys, tenants RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./internal/db/...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/db/testutil.go
git commit -m "feat(db): add WithTestDB helper for integration tests"
```

---

## Phase 4 — sqlc queries

### Task 10: Set up sqlc and generate typed queries

**Files:**
- Create: `sqlc.yaml`
- Create: `internal/db/sqlc/queries.sql`
- Create: `internal/db/sqlc/schema.sql`
- Create: `internal/db/sqlc/generated/` (generated)

- [ ] **Step 1: Verify sqlc is installed**

Run: `sqlc version`
Expected: prints a version string. If not installed: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`.

- [ ] **Step 2: Create `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/sqlc/queries.sql"
    schema: "internal/db/sqlc/schema.sql"
    gen:
      go:
        package: "sqlcgen"
        out: "internal/db/sqlc/generated"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
```

- [ ] **Step 3: Create `internal/db/sqlc/schema.sql` (mirror of the migration)**

Copy the exact contents of `internal/db/migrations/001_auth.up.sql` into `internal/db/sqlc/schema.sql`.

- [ ] **Step 4: Create `internal/db/sqlc/queries.sql`**

```sql
-- name: CreateTenant :one
INSERT INTO tenants (name, daily_sms_limit, monthly_budget)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY id;

-- name: SetTenantStatus :exec
UPDATE tenants SET status = $2, updated_at = now() WHERE id = $1;

-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, prefix, hash, name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys WHERE prefix = $1;

-- name: ListAPIKeysByTenant :many
SELECT * FROM api_keys WHERE tenant_id = $1 ORDER BY id DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: CreateAdminUser :one
INSERT INTO admin_users (email, password_hash, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAdminUserByEmail :one
SELECT * FROM admin_users WHERE email = $1;

-- name: ListAdminUsers :many
SELECT * FROM admin_users ORDER BY id;

-- name: CountAdminUsers :one
SELECT COUNT(*) FROM admin_users;

-- name: AppendAuditLog :exec
INSERT INTO audit_log (actor_id, action, target_type, target_id, metadata)
VALUES ($1, $2, $3, $4, $5);
```

- [ ] **Step 5: Generate**

Run: `sqlc generate`
Expected: creates files in `internal/db/sqlc/generated/` (db.go, models.go, queries.sql.go).

- [ ] **Step 6: Verify build**

Run: `go build ./...`
Expected: exit 0 (may need `go mod tidy` first).

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add sqlc.yaml internal/db/sqlc/ go.mod go.sum
git commit -m "feat(db): add sqlc config and generated queries"
```

---

## Phase 5 — Auth primitives (TDD)

### Task 11: Implement API key generation and hashing

**Files:**
- Create: `internal/auth/apikeyhash.go`
- Test: `internal/auth/apikeyhash_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/auth/apikeyhash_test.go
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
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/auth/... -v`
Expected: FAIL (undefined: GenerateToken, HashToken, VerifyToken).

- [ ] **Step 3: Implement `apikeyhash.go`**

```go
// internal/auth/apikeyhash.go
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
	tokenPrefix     = "sk_live_"
	tokenRandomLen  = 32 // bytes of entropy
	prefixVisibleLen = 12 // chars of the full token to persist as prefix
	fullTokenLen    = 51 // len("sk_live_") + 43 (base64url no padding of 32 bytes)
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
// the stored hash. Returns true on match.
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
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./internal/auth/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/apikeyhash.go internal/auth/apikeyhash_test.go
git commit -m "feat(auth): sha256+pepper api key hash and verify"
```

---

### Task 12: Implement JWT issue/parse

**Files:**
- Create: `internal/auth/token.go`
- Test: `internal/auth/token_test.go`

- [ ] **Step 1: Install jwt lib**

Run: `go get github.com/golang-jwt/jwt/v5@latest`

- [ ] **Step 2: Write the failing test**

```go
// internal/auth/token_test.go
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
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `go test ./internal/auth/... -run JWT -v`
Expected: FAIL (undefined: IssueJWT/ParseJWT/JWTClaims).

- [ ] **Step 4: Implement `token.go`**

```go
// internal/auth/token.go
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
```

- [ ] **Step 5: Run tests and verify they pass**

Run: `go test ./internal/auth/... -v`
Expected: PASS (all 4 tests: 3 new + previous hash tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/auth/token.go internal/auth/token_test.go
git commit -m "feat(auth): add JWT issue/parse with HS256"
```

---

## Phase 6 — Domain services (TDD with real Postgres)

### Task 13: Implement `internal/tenancy` — Tenant CRUD

**Files:**
- Create: `internal/tenancy/errors.go`
- Create: `internal/tenancy/tenant.go`
- Test: `internal/tenancy/tenant_test.go`

- [ ] **Step 1: Create `errors.go`**

```go
// internal/tenancy/errors.go
package tenancy

import "errors"

var (
	ErrTenantNotFound = errors.New("tenancy: tenant not found")
	ErrAPIKeyNotFound = errors.New("tenancy: api key not found")
	ErrAPIKeyRevoked  = errors.New("tenancy: api key revoked")
	ErrAPIKeyInvalid  = errors.New("tenancy: api key invalid")
)
```

- [ ] **Step 2: Write failing test `tenant_test.go`**

```go
// internal/tenancy/tenant_test.go
package tenancy_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func TestTenant_CreateAndGet(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()

	tt, err := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tt.ID == 0 {
		t.Fatal("id not assigned")
	}
	if tt.Name != "Acme" {
		t.Errorf("name = %q, want Acme", tt.Name)
	}
	if tt.Status != "active" {
		t.Errorf("status = %q, want active", tt.Status)
	}

	got, err := svc.GetTenant(ctx, tt.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != tt.ID || got.Name != "Acme" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestTenant_GetNotFound(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	if _, err := svc.GetTenant(context.Background(), 99999); err != tenancy.ErrTenantNotFound {
		t.Errorf("err = %v, want ErrTenantNotFound", err)
	}
}

func TestTenant_SuspendActivate(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()
	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "X"})

	if err := svc.SetStatus(ctx, tt.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetTenant(ctx, tt.ID)
	if got.Status != "suspended" {
		t.Errorf("status = %q, want suspended", got.Status)
	}
}
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `make migrate-test && go test ./internal/tenancy/... -v`
Expected: FAIL (undefined: tenancy.NewService, CreateTenantInput, etc.).

- [ ] **Step 4: Implement `tenant.go`**

```go
// internal/tenancy/tenant.go
package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID             int64
	Name           string
	Status         string
	DailySMSLimit  *int32
	MonthlyBudget  *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateTenantInput struct {
	Name          string
	DailySMSLimit *int32
	MonthlyBudget *string
}

// Service groups tenant and api-key operations backed by Postgres.
type Service struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool)}
}

func (s *Service) CreateTenant(ctx context.Context, in CreateTenantInput) (*Tenant, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	params := sqlcgen.CreateTenantParams{Name: in.Name}
	if in.DailySMSLimit != nil {
		params.DailySmsLimit = pgtype.Int4{Int32: *in.DailySMSLimit, Valid: true}
	}
	if in.MonthlyBudget != nil {
		params.MonthlyBudget = pgtype.Numeric{Valid: false}
		// Numeric parsing omitted for MVP — monthly budget stays NULL unless extended later.
	}
	row, err := s.q.CreateTenant(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return tenantFromRow(row), nil
}

func (s *Service) GetTenant(ctx context.Context, id int64) (*Tenant, error) {
	row, err := s.q.GetTenantByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return tenantFromRow(row), nil
}

func (s *Service) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.q.ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	out := make([]*Tenant, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenantFromRow(r))
	}
	return out, nil
}

func (s *Service) SetStatus(ctx context.Context, id int64, status string) error {
	if status != "active" && status != "suspended" {
		return fmt.Errorf("invalid status %q", status)
	}
	return s.q.SetTenantStatus(ctx, sqlcgen.SetTenantStatusParams{ID: id, Status: status})
}

func tenantFromRow(r sqlcgen.Tenant) *Tenant {
	t := &Tenant{
		ID:        r.ID,
		Name:      r.Name,
		Status:    r.Status,
		CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time,
	}
	if r.DailySmsLimit.Valid {
		v := r.DailySmsLimit.Int32
		t.DailySMSLimit = &v
	}
	return t
}
```

- [ ] **Step 5: Run tests**

Run: `make migrate-test && go test ./internal/tenancy/... -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/tenancy/
git commit -m "feat(tenancy): add Tenant CRUD service"
```

---

### Task 14: Implement API key operations in tenancy

**Files:**
- Create: `internal/tenancy/apikey.go`
- Test: `internal/tenancy/apikey_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/tenancy/apikey_test.go
package tenancy_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

const pepper = "test-pepper-lifecycle"

func TestAPIKey_Lifecycle(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()

	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "Acme"})

	issued, err := svc.IssueAPIKey(ctx, tt.ID, "laptop", pepper)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if issued.Token == "" || issued.Record.ID == 0 {
		t.Fatalf("bad issued: %+v", issued)
	}

	// Verify with correct token ⇒ returns tenant_id
	tenantID, err := svc.VerifyAPIKey(ctx, issued.Token, pepper)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tenantID != tt.ID {
		t.Errorf("tenantID = %d, want %d", tenantID, tt.ID)
	}

	// Revoke ⇒ verify rejects
	if err := svc.RevokeAPIKey(ctx, issued.Record.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.VerifyAPIKey(ctx, issued.Token, pepper); err != tenancy.ErrAPIKeyRevoked {
		t.Errorf("after revoke err = %v, want ErrAPIKeyRevoked", err)
	}
}

func TestAPIKey_VerifyUnknownPrefix(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	_, err := svc.VerifyAPIKey(context.Background(), "sk_live_"+"unknown0000000000000000000000000000000000", pepper)
	if err != tenancy.ErrAPIKeyNotFound {
		t.Errorf("err = %v, want ErrAPIKeyNotFound", err)
	}
}

func TestAPIKey_VerifyMalformed(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	_, err := svc.VerifyAPIKey(context.Background(), "not-a-key", pepper)
	if err != tenancy.ErrAPIKeyInvalid {
		t.Errorf("err = %v, want ErrAPIKeyInvalid", err)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/tenancy/... -run APIKey -v`
Expected: FAIL.

- [ ] **Step 3: Implement `apikey.go`**

```go
// internal/tenancy/apikey.go
package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/auth"
	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type APIKey struct {
	ID         int64
	TenantID   int64
	Prefix     string
	Name       *string
	Scopes     []string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type IssuedKey struct {
	Token  string   // full sk_live_... — show once
	Record *APIKey
}

func (s *Service) IssueAPIKey(ctx context.Context, tenantID int64, name, pepper string) (*IssuedKey, error) {
	if _, err := s.GetTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	token, prefix, err := auth.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("gen token: %w", err)
	}
	hash := auth.HashToken(token, pepper)
	var pgName pgtype.Text
	if name != "" {
		pgName = pgtype.Text{String: name, Valid: true}
	}
	row, err := s.q.CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		TenantID: tenantID,
		Prefix:   prefix,
		Hash:     hash,
		Name:     pgName,
	})
	if err != nil {
		return nil, fmt.Errorf("insert api_key: %w", err)
	}
	return &IssuedKey{Token: token, Record: apiKeyFromRow(row)}, nil
}

func (s *Service) VerifyAPIKey(ctx context.Context, token, pepper string) (int64, error) {
	prefix, ok := auth.PrefixOf(token)
	if !ok {
		return 0, ErrAPIKeyInvalid
	}
	row, err := s.q.GetAPIKeyByPrefix(ctx, prefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAPIKeyNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("lookup api_key: %w", err)
	}
	if row.RevokedAt.Valid {
		return 0, ErrAPIKeyRevoked
	}
	if !auth.VerifyToken(token, row.Hash, pepper) {
		return 0, ErrAPIKeyInvalid
	}
	// fire-and-forget touch
	go func(id int64) {
		ctxT, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.q.TouchAPIKey(ctxT, id)
	}(row.ID)
	return row.TenantID, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, id int64) error {
	return s.q.RevokeAPIKey(ctx, id)
}

func (s *Service) ListAPIKeys(ctx context.Context, tenantID int64) ([]*APIKey, error) {
	rows, err := s.q.ListAPIKeysByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	out := make([]*APIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, apiKeyFromRow(r))
	}
	return out, nil
}

func apiKeyFromRow(r sqlcgen.ApiKey) *APIKey {
	k := &APIKey{
		ID:        r.ID,
		TenantID:  r.TenantID,
		Prefix:    r.Prefix,
		Scopes:    r.Scopes,
		CreatedAt: r.CreatedAt.Time,
	}
	if r.Name.Valid {
		n := r.Name.String
		k.Name = &n
	}
	if r.LastUsedAt.Valid {
		t := r.LastUsedAt.Time
		k.LastUsedAt = &t
	}
	if r.RevokedAt.Valid {
		t := r.RevokedAt.Time
		k.RevokedAt = &t
	}
	return k
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tenancy/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tenancy/
git commit -m "feat(tenancy): issue/verify/revoke api keys"
```

---

### Task 15: Implement `internal/admin` — admin user ops

**Files:**
- Create: `internal/admin/errors.go`
- Create: `internal/admin/user.go`
- Test: `internal/admin/user_test.go`

- [ ] **Step 1: Create `errors.go`**

```go
// internal/admin/errors.go
package admin

import "errors"

var (
	ErrInvalidCredentials = errors.New("admin: invalid credentials")
	ErrAdminExists        = errors.New("admin: admin with that email already exists")
)
```

- [ ] **Step 2: Write failing test**

```go
// internal/admin/user_test.go
package admin_test

import (
	"context"
	"testing"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/db"
)

func TestAdmin_CreateAndLogin(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4) // low bcrypt cost for tests
	ctx := context.Background()

	u, err := svc.CreateAdmin(ctx, "a@b.com", "p1234567", "superadmin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("id not set")
	}

	// correct password ⇒ returns user
	got, err := svc.VerifyPassword(ctx, "a@b.com", "p1234567")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("id = %d, want %d", got.ID, u.ID)
	}

	// wrong password ⇒ ErrInvalidCredentials
	if _, err := svc.VerifyPassword(ctx, "a@b.com", "nope"); err != admin.ErrInvalidCredentials {
		t.Errorf("wrong pw err = %v, want ErrInvalidCredentials", err)
	}

	// unknown email ⇒ ErrInvalidCredentials (no enumeration)
	if _, err := svc.VerifyPassword(ctx, "no@x.com", "x"); err != admin.ErrInvalidCredentials {
		t.Errorf("unknown email err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAdmin_DuplicateEmail(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := admin.NewService(pool, 4)
	ctx := context.Background()
	_, _ = svc.CreateAdmin(ctx, "a@b.com", "x12345678", "superadmin")
	_, err := svc.CreateAdmin(ctx, "a@b.com", "y12345678", "operator")
	if err != admin.ErrAdminExists {
		t.Errorf("err = %v, want ErrAdminExists", err)
	}
}
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `go test ./internal/admin/... -v`
Expected: FAIL.

- [ ] **Step 4: Implement `user.go`**

```go
// internal/admin/user.go
package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int64
	Email     string
	Role      string
	CreatedAt time.Time
}

type Service struct {
	pool       *pgxpool.Pool
	q          *sqlcgen.Queries
	bcryptCost int
}

func NewService(pool *pgxpool.Pool, bcryptCost int) *Service {
	if bcryptCost < bcrypt.MinCost {
		bcryptCost = bcrypt.DefaultCost
	}
	return &Service{pool: pool, q: sqlcgen.New(pool), bcryptCost: bcryptCost}
}

func (s *Service) CreateAdmin(ctx context.Context, email, password, role string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(password) < 8 {
		return nil, fmt.Errorf("email required and password must be >= 8 chars")
	}
	if role != "superadmin" && role != "operator" {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	row, err := s.q.CreateAdminUser(ctx, sqlcgen.CreateAdminUserParams{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAdminExists
		}
		return nil, fmt.Errorf("insert admin: %w", err)
	}
	return &User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Service) VerifyPassword(ctx context.Context, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row, err := s.q.GetAdminUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup admin: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return &User{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Service) CountAdmins(ctx context.Context) (int64, error) {
	return s.q.CountAdminUsers(ctx)
}
```

- [ ] **Step 5: Install bcrypt**

Run: `go get golang.org/x/crypto/bcrypt@latest`

- [ ] **Step 6: Run tests**

Run: `go test ./internal/admin/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/admin/
git commit -m "feat(admin): create + verify admin users (bcrypt)"
```

---

## Phase 7 — HTTP primitives

### Task 16: Implement `internal/httpx` — response helpers and request ID

**Files:**
- Create: `internal/httpx/response.go`
- Create: `internal/httpx/reqid.go`
- Test: `internal/httpx/response_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/httpx/response_test.go
package httpx_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/S-martin-7/sms/internal/httpx"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.WriteJSON(w, 201, map[string]string{"a": "b"})
	if w.Code != 201 {
		t.Errorf("code = %d, want 201", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	var got map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["a"] != "b" {
		t.Errorf("body = %v", got)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.WriteError(w, 401, "unauthorized", "no api key")
	if w.Code != 401 {
		t.Errorf("code = %d, want 401", w.Code)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Error.Code != "unauthorized" || got.Error.Message != "no api key" {
		t.Errorf("body = %+v", got)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/httpx/... -v`
Expected: FAIL.

- [ ] **Step 3: Implement `response.go`**

```go
// internal/httpx/response.go
package httpx

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorPayload struct {
	Error errorBody `json:"error"`
}
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, errorPayload{Error: errorBody{Code: code, Message: msg}})
}
```

- [ ] **Step 4: Implement `reqid.go`**

```go
// internal/httpx/reqid.go
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type reqIDKey struct{}

// RequestID middleware assigns X-Request-ID (from incoming header or generates one)
// and stores it in the context. Down-stream handlers can retrieve it with RequestIDFrom.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			id = hex.EncodeToString(buf)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), reqIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(reqIDKey{}).(string); ok {
		return v
	}
	return ""
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/httpx/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpx/
git commit -m "feat(httpx): add WriteJSON/WriteError and request-id middleware"
```

---

### Task 17: Implement API-key middleware

**Files:**
- Create: `internal/auth/middleware/apikey.go`
- Test: `internal/auth/middleware/apikey_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/auth/middleware/apikey_test.go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func TestAPIKeyMiddleware(t *testing.T) {
	pool := db.WithTestDB(t)
	svc := tenancy.NewService(pool)
	ctx := context.Background()
	tt, _ := svc.CreateTenant(ctx, tenancy.CreateTenantInput{Name: "T1"})
	issued, _ := svc.IssueAPIKey(ctx, tt.ID, "test", "pepper-mw")

	handler := middleware.APIKey(svc, "pepper-mw")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := middleware.TenantIDFrom(r.Context()); got != tt.ID {
				t.Errorf("ctx tenantID = %d, want %d", got, tt.ID)
			}
			w.WriteHeader(204)
		}))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", issued.Token, 204},
		{"missing", "", 401},
		{"malformed", "garbage", 401},
		{"unknown", "sk_live_"+"0000000000000000000000000000000000000000000", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				req.Header.Set("X-API-Key", c.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("code = %d, want %d", rec.Code, c.want)
			}
		})
	}

	// revoked
	_ = svc.RevokeAPIKey(ctx, issued.Record.ID)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", issued.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("after revoke code = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/auth/middleware/... -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement `apikey.go`**

```go
// internal/auth/middleware/apikey.go
package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/tenancy"
)

type tenantKey struct{}

// APIKey verifies the X-API-Key header against the tenancy service and
// injects the tenant id into the request context.
func APIKey(svc *tenancy.Service, pepper string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-API-Key")
			if token == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing X-API-Key")
				return
			}
			tenantID, err := svc.VerifyAPIKey(r.Context(), token, pepper)
			if err != nil {
				switch {
				case errors.Is(err, tenancy.ErrAPIKeyInvalid),
					errors.Is(err, tenancy.ErrAPIKeyNotFound),
					errors.Is(err, tenancy.ErrAPIKeyRevoked):
					httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid api key")
				default:
					httpx.WriteError(w, http.StatusInternalServerError, "internal", "auth lookup failed")
				}
				return
			}
			ctx := context.WithValue(r.Context(), tenantKey{}, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantIDFrom retrieves the tenant id set by APIKey. Returns 0 if absent.
func TenantIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(tenantKey{}).(int64); ok {
		return v
	}
	return 0
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/auth/middleware/... -v`
Expected: PASS (5 subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware/
git commit -m "feat(auth/mw): add X-API-Key middleware"
```

---

### Task 18: Implement admin JWT middleware

**Files:**
- Create: `internal/auth/middleware/adminjwt.go`
- Test: `internal/auth/middleware/adminjwt_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/auth/middleware/adminjwt_test.go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/S-martin-7/sms/internal/auth"
	"github.com/S-martin-7/sms/internal/auth/middleware"
)

func TestAdminJWTMiddleware(t *testing.T) {
	secret := []byte("jwt-mw-secret")
	tok, _, _ := auth.IssueJWT(secret, auth.JWTClaims{Sub: 7, Role: "superadmin"}, time.Hour)

	handler := middleware.AdminJWT(secret)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, role := middleware.AdminIDFrom(r.Context()), middleware.AdminRoleFrom(r.Context())
			if id != 7 || role != "superadmin" {
				t.Errorf("ctx id/role = %d/%q", id, role)
			}
			w.WriteHeader(204)
		}))

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer " + tok, 204},
		{"missing", "", 401},
		{"no-prefix", tok, 401},
		{"bad", "Bearer garbage", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("code = %d, want %d", rec.Code, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/auth/middleware/... -run AdminJWT -v`
Expected: FAIL.

- [ ] **Step 3: Implement `adminjwt.go`**

```go
// internal/auth/middleware/adminjwt.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/S-martin-7/sms/internal/auth"
	"github.com/S-martin-7/sms/internal/httpx"
)

type adminIDKey struct{}
type adminRoleKey struct{}

// AdminJWT parses an Authorization: Bearer <jwt>, verifies it with secret,
// and stores (adminID, role) in the request context.
func AdminJWT(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(hdr, "Bearer ") {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing bearer")
				return
			}
			tok := strings.TrimPrefix(hdr, "Bearer ")
			claims, err := auth.ParseJWT(secret, tok)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), adminIDKey{}, claims.Sub)
			ctx = context.WithValue(ctx, adminRoleKey{}, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AdminIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(adminIDKey{}).(int64); ok {
		return v
	}
	return 0
}

func AdminRoleFrom(ctx context.Context) string {
	if v, ok := ctx.Value(adminRoleKey{}).(string); ok {
		return v
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/auth/middleware/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware/adminjwt.go internal/auth/middleware/adminjwt_test.go
git commit -m "feat(auth/mw): add Bearer JWT admin middleware"
```

---

## Phase 8 — HTTP server

### Task 19: Implement router and handlers

**Files:**
- Create: `internal/httpx/router.go`
- Create: `internal/httpx/handlers_admin.go`
- Create: `internal/httpx/handlers_public.go`

- [ ] **Step 1: Install chi**

Run: `go get github.com/go-chi/chi/v5@latest`

- [ ] **Step 2: Create `handlers_admin.go` (login)**

```go
// internal/httpx/handlers_admin.go
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Role      string    `json:"role"`
}

// LoginHandler validates credentials via admin.Service and issues a JWT.
func LoginHandler(svc *admin.Service, jwtSecret []byte, ttl time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in loginReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		u, err := svc.VerifyPassword(r.Context(), in.Email, in.Password)
		if err != nil {
			if errors.Is(err, admin.ErrInvalidCredentials) {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal", "login failed")
			return
		}
		tok, exp, err := auth.IssueJWT(jwtSecret, auth.JWTClaims{Sub: u.ID, Role: u.Role}, ttl)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "token issue failed")
			return
		}
		WriteJSON(w, http.StatusOK, loginResp{Token: tok, ExpiresAt: exp, Role: u.Role})
	}
}
```

- [ ] **Step 3: Create `handlers_public.go` (ping)**

```go
// internal/httpx/handlers_public.go
package httpx

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/auth/middleware"
)

type pingResp struct {
	OK       bool      `json:"ok"`
	TenantID int64     `json:"tenant_id"`
	At       time.Time `json:"at"`
}

// PingHandler returns the caller's tenant id — auth pipeline smoke test.
func PingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, pingResp{
			OK:       true,
			TenantID: middleware.TenantIDFrom(r.Context()),
			At:       time.Now().UTC(),
		})
	}
}
```

- [ ] **Step 4: Create `router.go`**

```go
// internal/httpx/router.go
package httpx

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/go-chi/chi/v5"
)

type RouterDeps struct {
	AdminSvc     *admin.Service
	TenancySvc   *tenancy.Service
	JWTSecret    []byte
	JWTTTL       time.Duration
	APIKeyPepper string
}

// NewRouter mounts /admin/login and /v1/ping.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestID)

	r.Post("/admin/login", LoginHandler(d.AdminSvc, d.JWTSecret, d.JWTTTL))

	r.Group(func(r chi.Router) {
		r.Use(middleware.APIKey(d.TenancySvc, d.APIKeyPepper))
		r.Get("/v1/ping", PingHandler())
	})

	return r
}
```

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/httpx/
git commit -m "feat(httpx): add router with /admin/login and /v1/ping"
```

---

### Task 20: Implement `cmd/server/main.go`

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: Create `main.go`**

```go
// cmd/server/main.go
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/logger"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.LogLevel, cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("db open failed")
	}
	defer pool.Close()

	tenancySvc := tenancy.NewService(pool)
	adminSvc := admin.NewService(pool, cfg.BcryptCost)

	handler := httpx.NewRouter(httpx.RouterDeps{
		AdminSvc:     adminSvc,
		TenancySvc:   tenancySvc,
		JWTSecret:    []byte(cfg.JWTSecret),
		JWTTTL:       time.Duration(cfg.JWTTTLHours) * time.Hour,
		APIKeyPepper: cfg.APIKeyPepper,
	})

	srv := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.BindAddr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("listen")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown")
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/server`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/
git commit -m "feat(server): wire cmd/server entrypoint"
```

---

## Phase 9 — CLI `smsctl`

### Task 21: Implement `cmd/smsctl` skeleton + `migrate` subcommand

**Files:**
- Create: `cmd/smsctl/main.go`

- [ ] **Step 1: Create `main.go`**

```go
// cmd/smsctl/main.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	rest := os.Args[2:]
	var err error
	switch cmd {
	case "migrate":
		err = runMigrate(rest)
	case "admin":
		err = runAdmin(rest)
	case "tenant":
		err = runTenant(rest)
	case "key":
		err = runKey(rest)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `smsctl — SMS gateway admin CLI

Commands:
  migrate up|down|version
  admin create --email X --password Y [--role superadmin|operator]
  admin list
  tenant create --name "Acme" [--daily-limit N]
  tenant list
  tenant suspend --id N
  tenant activate --id N
  key issue --tenant-id N [--name "label"]
  key list --tenant-id N
  key revoke --id N`)
}

func runMigrate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl migrate up|down|version")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "up":
		if err := db.Migrate(cfg.DatabaseURL, "up"); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		fmt.Println("migrate up: ok")
	case "down":
		if err := db.Migrate(cfg.DatabaseURL, "down"); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		fmt.Println("migrate down: ok")
	case "version":
		v, dirty, err := db.Version(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", v, dirty)
	default:
		return fmt.Errorf("unknown migrate subcommand: %s", args[0])
	}
	_ = context.Background()
	return nil
}

// --- stubs; implemented in next tasks ---

func runAdmin(args []string) error  { return fmt.Errorf("admin: not yet implemented") }
func runTenant(args []string) error { return fmt.Errorf("tenant: not yet implemented") }
func runKey(args []string) error    { return fmt.Errorf("key: not yet implemented") }
```

- [ ] **Step 2: Smoke-test against dev DB**

First set env or use Makefile:
```bash
export DATABASE_URL='postgres://sms:sms@localhost:5432/sms?sslmode=disable'
export JWT_SECRET='x'
export API_KEY_PEPPER='x'
go run ./cmd/smsctl migrate up
go run ./cmd/smsctl migrate version
```
Expected: `migrate up: ok`, then `version=1 dirty=false`.

Rollback:
```bash
go run ./cmd/smsctl migrate down
go run ./cmd/smsctl migrate version
```
Expected: `version=0 dirty=false`.

Then re-apply for subsequent tasks:
```bash
go run ./cmd/smsctl migrate up
```

- [ ] **Step 3: Commit**

```bash
git add cmd/smsctl/
git commit -m "feat(smsctl): skeleton + migrate subcommand"
```

---

### Task 22: Implement `smsctl admin` subcommand

**Files:**
- Modify: `cmd/smsctl/main.go` (replace `runAdmin` stub)
- Create: `cmd/smsctl/admin.go`

- [ ] **Step 1: Create `admin.go`**

```go
// cmd/smsctl/admin.go
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
)

func runAdmin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl admin create|list")
	}
	sub, rest := args[0], args[1:]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	svc := admin.NewService(pool, cfg.BcryptCost)

	switch sub {
	case "create":
		fs := flag.NewFlagSet("admin create", flag.ExitOnError)
		email := fs.String("email", "", "email")
		password := fs.String("password", "", "password")
		role := fs.String("role", "", "superadmin|operator (default auto)")
		_ = fs.Parse(rest)
		if *email == "" || *password == "" {
			return fmt.Errorf("--email and --password are required")
		}
		chosen := *role
		if chosen == "" {
			n, _ := svc.CountAdmins(ctx)
			if n == 0 {
				chosen = "superadmin"
			} else {
				chosen = "operator"
			}
		}
		u, err := svc.CreateAdmin(ctx, *email, *password, chosen)
		if err != nil {
			return err
		}
		fmt.Printf("admin created: id=%d email=%s role=%s\n", u.ID, u.Email, u.Role)
		return nil
	case "list":
		// Minimal listing via raw pool — no service method for MVP.
		rows, err := pool.Query(ctx, `SELECT id, email, role, created_at FROM admin_users ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var email, role string
			var created time.Time
			if err := rows.Scan(&id, &email, &role, &created); err != nil {
				return err
			}
			fmt.Printf("%d\t%s\t%s\t%s\n", id, email, role, created.Format(time.RFC3339))
		}
		return rows.Err()
	default:
		return fmt.Errorf("unknown admin subcommand: %s", sub)
	}
}
```

- [ ] **Step 2: Remove the stub in `main.go`**

Delete the line `func runAdmin(args []string) error  { return fmt.Errorf("admin: not yet implemented") }` from `cmd/smsctl/main.go`.

- [ ] **Step 3: Smoke-test**

```bash
go run ./cmd/smsctl admin create --email a@b.com --password p1234567
go run ./cmd/smsctl admin list
```
Expected: prints `admin created: id=1 email=a@b.com role=superadmin`, then `list` shows the row.

- [ ] **Step 4: Commit**

```bash
git add cmd/smsctl/
git commit -m "feat(smsctl): admin create/list subcommands"
```

---

### Task 23: Implement `smsctl tenant` subcommand

**Files:**
- Create: `cmd/smsctl/tenant.go`
- Modify: `cmd/smsctl/main.go` (remove `runTenant` stub)

- [ ] **Step 1: Create `tenant.go`**

```go
// cmd/smsctl/tenant.go
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func runTenant(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl tenant create|list|suspend|activate")
	}
	sub, rest := args[0], args[1:]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	svc := tenancy.NewService(pool)

	switch sub {
	case "create":
		fs := flag.NewFlagSet("tenant create", flag.ExitOnError)
		name := fs.String("name", "", "tenant name")
		daily := fs.Int("daily-limit", 0, "daily SMS limit (0=unlimited)")
		_ = fs.Parse(rest)
		if *name == "" {
			return fmt.Errorf("--name required")
		}
		in := tenancy.CreateTenantInput{Name: *name}
		if *daily > 0 {
			v := int32(*daily)
			in.DailySMSLimit = &v
		}
		t, err := svc.CreateTenant(ctx, in)
		if err != nil {
			return err
		}
		fmt.Printf("tenant created: id=%d name=%s status=%s\n", t.ID, t.Name, t.Status)
		return nil
	case "list":
		ts, err := svc.ListTenants(ctx)
		if err != nil {
			return err
		}
		for _, t := range ts {
			fmt.Printf("%d\t%s\t%s\t%s\n", t.ID, t.Name, t.Status, t.CreatedAt.Format(time.RFC3339))
		}
		return nil
	case "suspend", "activate":
		fs := flag.NewFlagSet("tenant "+sub, flag.ExitOnError)
		id := fs.Int64("id", 0, "tenant id")
		_ = fs.Parse(rest)
		if *id == 0 {
			return fmt.Errorf("--id required")
		}
		target := "suspended"
		if sub == "activate" {
			target = "active"
		}
		if err := svc.SetStatus(ctx, *id, target); err != nil {
			return err
		}
		fmt.Printf("tenant %d → %s\n", *id, target)
		return nil
	default:
		return fmt.Errorf("unknown tenant subcommand: %s", sub)
	}
}
```

- [ ] **Step 2: Remove stub**

Delete `func runTenant(args []string) error { return fmt.Errorf("tenant: not yet implemented") }` from `cmd/smsctl/main.go`.

- [ ] **Step 3: Smoke-test**

```bash
go run ./cmd/smsctl tenant create --name Acme
go run ./cmd/smsctl tenant list
go run ./cmd/smsctl tenant suspend --id 1
go run ./cmd/smsctl tenant activate --id 1
```
Expected: each prints a confirming line.

- [ ] **Step 4: Commit**

```bash
git add cmd/smsctl/
git commit -m "feat(smsctl): tenant create/list/suspend/activate"
```

---

### Task 24: Implement `smsctl key` subcommand

**Files:**
- Create: `cmd/smsctl/key.go`
- Modify: `cmd/smsctl/main.go` (remove `runKey` stub)

- [ ] **Step 1: Create `key.go`**

```go
// cmd/smsctl/key.go
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/S-martin-7/sms/internal/config"
	"github.com/S-martin-7/sms/internal/db"
	"github.com/S-martin-7/sms/internal/tenancy"
)

func runKey(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: smsctl key issue|list|revoke")
	}
	sub, rest := args[0], args[1:]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	svc := tenancy.NewService(pool)

	switch sub {
	case "issue":
		fs := flag.NewFlagSet("key issue", flag.ExitOnError)
		tid := fs.Int64("tenant-id", 0, "tenant id")
		name := fs.String("name", "", "label")
		_ = fs.Parse(rest)
		if *tid == 0 {
			return fmt.Errorf("--tenant-id required")
		}
		issued, err := svc.IssueAPIKey(ctx, *tid, *name, cfg.APIKeyPepper)
		if err != nil {
			return err
		}
		fmt.Println("API KEY (shown ONCE — copy now):")
		fmt.Println(issued.Token)
		fmt.Printf("\nid=%d prefix=%s tenant_id=%d\n", issued.Record.ID, issued.Record.Prefix, issued.Record.TenantID)
		return nil
	case "list":
		fs := flag.NewFlagSet("key list", flag.ExitOnError)
		tid := fs.Int64("tenant-id", 0, "tenant id")
		_ = fs.Parse(rest)
		if *tid == 0 {
			return fmt.Errorf("--tenant-id required")
		}
		keys, err := svc.ListAPIKeys(ctx, *tid)
		if err != nil {
			return err
		}
		for _, k := range keys {
			state := "active"
			if k.RevokedAt != nil {
				state = "revoked@" + k.RevokedAt.Format(time.RFC3339)
			}
			name := ""
			if k.Name != nil {
				name = *k.Name
			}
			fmt.Printf("%d\t%s\t%s\t%s\n", k.ID, k.Prefix, name, state)
		}
		return nil
	case "revoke":
		fs := flag.NewFlagSet("key revoke", flag.ExitOnError)
		id := fs.Int64("id", 0, "key id")
		_ = fs.Parse(rest)
		if *id == 0 {
			return fmt.Errorf("--id required")
		}
		if err := svc.RevokeAPIKey(ctx, *id); err != nil {
			return err
		}
		fmt.Printf("key %d revoked\n", *id)
		return nil
	default:
		return fmt.Errorf("unknown key subcommand: %s", sub)
	}
}
```

- [ ] **Step 2: Remove stub**

Delete `func runKey(args []string) error    { return fmt.Errorf("key: not yet implemented") }` from `cmd/smsctl/main.go`.

- [ ] **Step 3: Smoke-test**

```bash
go run ./cmd/smsctl key issue --tenant-id 1 --name laptop
# Copy the printed sk_live_... token for the e2e test
go run ./cmd/smsctl key list --tenant-id 1
```
Expected: printed token, list shows active row.

- [ ] **Step 4: Commit**

```bash
git add cmd/smsctl/
git commit -m "feat(smsctl): key issue/list/revoke"
```

---

## Phase 10 — End-to-end verification

### Task 25: End-to-end smoke test (manual)

No code — this is a manual verification step that runs the whole pipeline.

- [ ] **Step 1: Ensure clean DB state**

```bash
go run ./cmd/smsctl migrate down
go run ./cmd/smsctl migrate up
go run ./cmd/smsctl admin create --email a@b.com --password p1234567
go run ./cmd/smsctl tenant create --name Acme
```

- [ ] **Step 2: Issue key and capture the token**

```bash
go run ./cmd/smsctl key issue --tenant-id 1 --name e2e
# Save the printed token into $TOKEN (manual copy-paste in your shell)
```

- [ ] **Step 3: Start the server in one shell**

```bash
go run ./cmd/server
```
Expected: `server listening addr=127.0.0.1:7300` log line.

- [ ] **Step 4: Ping with valid key**

```bash
curl -s -H "X-API-Key: $TOKEN" http://127.0.0.1:7300/v1/ping
```
Expected: `{"ok":true,"tenant_id":1,"at":"..."}`.

- [ ] **Step 5: Ping with bad key → 401**

```bash
curl -s -o /dev/null -w "%{http_code}\n" -H "X-API-Key: bad" http://127.0.0.1:7300/v1/ping
```
Expected: `401`.

- [ ] **Step 6: Revoke then ping → 401**

```bash
go run ./cmd/smsctl key revoke --id 1
curl -s -o /dev/null -w "%{http_code}\n" -H "X-API-Key: $TOKEN" http://127.0.0.1:7300/v1/ping
```
Expected: `401`.

- [ ] **Step 7: Login → JWT**

```bash
curl -s -X POST -H "Content-Type: application/json" \
  -d '{"email":"a@b.com","password":"p1234567"}' \
  http://127.0.0.1:7300/admin/login
```
Expected: JSON with non-empty `token`, `expires_at`, `role=superadmin`.

- [ ] **Step 8: Login with bad creds → 401**

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST -H "Content-Type: application/json" \
  -d '{"email":"a@b.com","password":"wrong"}' \
  http://127.0.0.1:7300/admin/login
```
Expected: `401`.

- [ ] **Step 9: Commit — nothing to commit. Mark this task done by noting the successful verification in the next commit (or just mark the checkbox and proceed).**

---

### Task 26: Final `go test ./...` pass

- [ ] **Step 1: Ensure test DB is fresh**

```bash
make migrate-test
```

- [ ] **Step 2: Run full suite**

```bash
make test
```
Expected: all packages PASS.

- [ ] **Step 3: If green, tag the milestone commit**

```bash
git log --oneline -1
# If the working tree is clean and tests pass:
git tag -a scaffold-auth-v1 -m "scaffold + auth layer complete"
```

- [ ] **Step 4: Announce completion in the conversation.**

---

## Coverage map (spec → task)

- Repo scaffold & Makefile → Tasks 1, 3
- docker-compose Postgres + test DB → Task 2
- Migration 001_auth incremental → Task 7
- Migration runner → Task 8
- Config env loader → Task 4
- Logger zerolog → Task 5
- Pgxpool → Task 6
- Test helper WithTestDB → Task 9
- sqlc queries → Task 10
- API key hash (sha256+pepper) → Task 11
- JWT → Task 12
- Tenant service → Task 13
- API key service → Task 14
- Admin user service (bcrypt) → Task 15
- HTTP response/reqid → Task 16
- X-API-Key middleware → Task 17
- Admin JWT middleware → Task 18
- Router + handlers (`/admin/login`, `/v1/ping`) → Task 19
- cmd/server → Task 20
- smsctl migrate → Task 21
- smsctl admin → Task 22
- smsctl tenant → Task 23
- smsctl key → Task 24
- End-to-end smoke → Task 25
- `go test ./...` → Task 26

All 11 acceptance criteria in the spec are exercised by Tasks 21–26 (manual smoke) and the test suites in Tasks 4, 11–18.
