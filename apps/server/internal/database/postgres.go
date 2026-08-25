package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL, applicationName string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["application_name"] = applicationName

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return pool, nil
}

// OpenApplication applies pending schema changes before opening an application pool.
func OpenApplication(ctx context.Context, databaseURL, applicationName string) (*pgxpool.Pool, error) {
	if err := MigrateUp(databaseURL); err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL schema: %w", err)
	}
	connectContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return Open(connectContext, databaseURL, applicationName)
}

func ValidateRuntimeRole(ctx context.Context, pool *pgxpool.Pool, expectedDatabase string) error {
	var user, database string
	var superuser, createDatabase, createRole, bypassRLS, databaseCreate, schemaCreate bool
	err := pool.QueryRow(ctx, `
SELECT current_user,
       current_database(),
       role.rolsuper,
       role.rolcreatedb,
       role.rolcreaterole,
       role.rolbypassrls,
       has_database_privilege(current_user, current_database(), 'CREATE'),
       has_schema_privilege(current_user, 'public', 'CREATE')
FROM pg_roles AS role
WHERE role.rolname = current_user`).Scan(
		&user, &database, &superuser, &createDatabase, &createRole, &bypassRLS, &databaseCreate, &schemaCreate,
	)
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL runtime role: %w", err)
	}
	if database != expectedDatabase {
		return fmt.Errorf("PostgreSQL connected to %q, expected %q", database, expectedDatabase)
	}
	if superuser || createDatabase || createRole || bypassRLS || databaseCreate || schemaCreate {
		return fmt.Errorf("PostgreSQL runtime role %q has schema or administrative privileges", user)
	}
	return nil
}
