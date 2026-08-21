package database_test

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	authn "github.com/Actify/echonote/apps/server/internal/auth"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestIdentityMigrationBackfillsAndClaimsExistingUser(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_MIGRATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_MIGRATION_TEST_DATABASE_URL to run the migration backfill test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	version, dirty, err := database.MigrationVersion(databaseURL)
	if err != nil || dirty || version < 7 {
		t.Fatalf("migration version=%d dirty=%t err=%v", version, dirty, err)
	}
	if err := database.MigrateDown(databaseURL, int(version-6)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.MigrateUp(databaseURL); err != nil {
			t.Errorf("restore migration version: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-identity-migration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := migrationUUID(t)
	var episodeID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO episodes (user_id, title, resolve_status)
		VALUES ($1, 'Existing Phase 8 Episode', 'completed')
		RETURNING id
	`, userID).Scan(&episodeID); err != nil {
		t.Fatal(err)
	}

	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM users WHERE id = $1", userID).Scan(&status); err != nil || status != "placeholder" {
		t.Fatalf("backfilled user status=%q err=%v", status, err)
	}
	passwordHash, err := authn.HashPassword("migration test password", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewAuthRepository(pool).ClaimUser(ctx, userID, "migrated_user", "migrated_user", passwordHash); err != nil {
		t.Fatal(err)
	}
	episodes, total, err := repository.NewLibraryRepository(pool).List(ctx, userID, 10, 0)
	if err != nil || total != 1 || len(episodes) != 1 || episodes[0].ID != episodeID {
		t.Fatalf("existing data total=%d episodes=%+v err=%v", total, episodes, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM episodes WHERE id = $1", episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
}

func migrationUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: value, Valid: true}
}
