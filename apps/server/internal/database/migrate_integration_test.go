package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
)

func TestTranscriptionServiceMigration(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_MIGRATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_MIGRATION_TEST_DATABASE_URL to run the migration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	version, dirty, err := database.MigrationVersion(databaseURL)
	if err != nil || dirty || version < 10 {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-transcription-migration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var issuerColumn, notesTable bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'auth_issuer'
	)`).Scan(&issuerColumn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.notes') IS NOT NULL").Scan(&notesTable); err != nil {
		t.Fatal(err)
	}
	if !issuerColumn || notesTable {
		t.Fatalf("auth_issuer=%t notes_table=%t", issuerColumn, notesTable)
	}
}
