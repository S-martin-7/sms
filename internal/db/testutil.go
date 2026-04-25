package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testLockKey serialises WithTestDB callers across ALL test packages in
// the same Postgres database. Different go test binaries running in
// parallel (e.g. go test ./... spawning one per package) would otherwise
// TRUNCATE each other's rows.
const testLockKey = 8274567128347 // arbitrary constant

// WithTestDB opens a pool against DATABASE_URL_TEST, acquires a postgres
// advisory lock, truncates all domain tables, and returns the pool.
// The test is skipped if the env var is unset. Lock is released when
// the pool is closed (via t.Cleanup).
func WithTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Acquire an exclusive lock on a dedicated connection so it persists
	// until we release it. The lock blocks other test packages while we run.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire conn: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(testLockKey)); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("advisory lock: %v", err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(testLockKey))
		conn.Release()
		pool.Close()
	})

	_, err = pool.Exec(ctx, `TRUNCATE events, inbound_messages, inbound_numbers, webhook_deliveries, webhook_endpoints, messages, audit_log, admin_users, api_keys, tenants RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}
