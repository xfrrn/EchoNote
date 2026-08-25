package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenApplicationInitializesEmptyDatabase(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_SCHEMA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_SCHEMA_TEST_DATABASE_URL to run the schema initialization test")
	}
	version, dirty, err := MigrationVersion(databaseURL)
	if err != nil || dirty {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	if version > 0 {
		t.Skip("ECHONOTE_SCHEMA_TEST_DATABASE_URL must point to an empty database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := OpenApplication(ctx, databaseURL, "echonote-schema-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tables int
	if err := pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_catalog.pg_tables
WHERE schemaname = 'public'
  AND tablename = ANY($1::text[])
`, []string{
		"jobs", "job_events", "episodes", "episode_sources", "episode_identity_keys", "imports",
		"transcription_runs", "transcription_chunks", "transcription_events", "transcript_versions", "transcript_speakers", "transcript_segments",
		"users",
	}).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 13 {
		t.Fatalf("initialized tables=%d, want 13", tables)
	}
}

func TestValidateRuntimeRoleRejectsSuperuser(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := Open(ctx, databaseURL, "echonote-runtime-role-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var superuser bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&superuser); err != nil {
		t.Fatal(err)
	}
	if !superuser {
		t.Skip("test connection is not a superuser")
	}
	if err := ValidateRuntimeRole(ctx, pool, "echonote_test"); err == nil || !strings.Contains(err.Error(), "administrative privileges") {
		t.Fatalf("expected superuser rejection, got %v", err)
	}
}
