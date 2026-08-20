package service

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

type countingResolver struct {
	calls int
}

func (*countingResolver) CanResolve(string) bool { return true }

func (r *countingResolver) Resolve(_ context.Context, rawURL string) (*domain.ResolvedEpisode, error) {
	r.calls++
	return &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "Resolved Capture",
		CanonicalURL: rawURL, AudioURL: rawURL,
	}, nil
}

func TestResolveImportHandlerProcessesPendingCaptureOnce(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-import-service-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomServiceUUID(t)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
	}()

	imports := repository.NewImportRepository(pool)
	capture, err := repository.NewNotesRepository(pool).CaptureURL(
		ctx,
		userID,
		randomServiceUUID(t),
		"https://cdn.example.com/pending-capture.mp3",
		"resolve this capture",
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &countingResolver{}
	handler := NewResolveImportHandler(imports, resolver)
	job := db.Job{UserID: userID, EntityType: "import", EntityID: capture.ImportID}

	if err := handler(ctx, job); err != nil {
		t.Fatal(err)
	}
	status, err := imports.Get(ctx, userID, capture.ImportID)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || status.EpisodeResolveStatus == nil || *status.EpisodeResolveStatus != "completed" {
		t.Fatalf("resolver calls=%d status=%+v", resolver.calls, status)
	}
	var title string
	if err := pool.QueryRow(ctx, "SELECT title FROM episodes WHERE id = $1", capture.Note.EpisodeID).Scan(&title); err != nil || title != "Resolved Capture" {
		t.Fatalf("title=%q err=%v", title, err)
	}
	if err := handler(ctx, job); err != nil || resolver.calls != 1 {
		t.Fatalf("idempotent handler calls=%d err=%v", resolver.calls, err)
	}
}

func randomServiceUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	id := pgtype.UUID{Valid: true}
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		t.Fatal(err)
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	return id
}
