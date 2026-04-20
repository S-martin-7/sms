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

// pgxDSN converts a plain postgres DSN to the pgx/v5 driver URL.
func pgxDSN(dsn string) string {
	if len(dsn) >= 11 && dsn[:11] == "postgres://" {
		return "pgx5://" + dsn[11:]
	}
	return dsn
}
