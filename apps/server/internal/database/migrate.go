package database

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/Actify/echonote/apps/server/migrations"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func MigrateUp(databaseURL string) error {
	instance, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	err = instance.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return closeMigrator(instance, err)
}

func MigrateDown(databaseURL string, steps int) error {
	if steps < 1 {
		return fmt.Errorf("down migration steps must be positive")
	}
	instance, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	err = instance.Steps(-steps)
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return closeMigrator(instance, err)
}

func MigrationVersion(databaseURL string) (version uint, dirty bool, err error) {
	instance, err := newMigrator(databaseURL)
	if err != nil {
		return 0, false, err
	}
	version, dirty, err = instance.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		err = nil
	}
	err = closeMigrator(instance, err)
	return version, dirty, err
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL for migrations: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" && parsed.Scheme != "pgx5" {
		return nil, fmt.Errorf("DATABASE_URL must use postgres, postgresql, or pgx5 scheme")
	}
	query := parsed.Query()
	query.Del("pool_max_conns")
	parsed.RawQuery = query.Encode()
	parsed.Scheme = "pgx5"

	instance, err := migrate.NewWithSourceInstance("iofs", source, parsed.String())
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return instance, nil
}

func closeMigrator(instance *migrate.Migrate, prior error) error {
	sourceErr, databaseErr := instance.Close()
	return errors.Join(prior, sourceErr, databaseErr)
}
