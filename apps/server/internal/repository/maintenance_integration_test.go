package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRetentionPreviewsThenDeletesOnlyExpiredSystemRecords(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-retention-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomUUID(t)
	defer ensureTestUsers(t, pool, userID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	completedOld := insertRetentionJob(t, ctx, pool, userID, "succeeded", now.Add(-31*24*time.Hour))
	failedOld := insertRetentionJob(t, ctx, pool, userID, "failed", now.Add(-91*24*time.Hour))
	completedRecent := insertRetentionJob(t, ctx, pool, userID, "canceled", now.Add(-29*24*time.Hour))
	insertRetentionJob(t, ctx, pool, userID, "failed", now.Add(-89*24*time.Hour))
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (user_id, type, entity_type, entity_id)
        VALUES ($1, 'retention_queued', 'retention_test', gen_random_uuid())`, userID); err != nil {
		t.Fatal(err)
	}
	for _, jobID := range []pgtype.UUID{completedOld, failedOld, completedRecent} {
		if _, err := pool.Exec(ctx, `INSERT INTO job_events (job_id, event_type, stage)
            VALUES ($1, 'retention_test', 'completed')`, jobID); err != nil {
			t.Fatal(err)
		}
	}
	var failedEpisodeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO episodes (user_id, title, resolve_status)
		VALUES ($1, 'Failed retained import', 'failed') RETURNING id`, userID).Scan(&failedEpisodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO imports (user_id, submitted_url, job_id, episode_id)
		VALUES ($1, 'https://cdn.example.com/expired-import.mp3', $2, $3)`, userID, failedOld, failedEpisodeID); err != nil {
		t.Fatal(err)
	}
	var episodeID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO episodes (user_id, title, resolve_status)
        VALUES ($1, 'Retained import', 'completed') RETURNING id`, userID).Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO imports (user_id, submitted_url, job_id, episode_id)
        VALUES ($1, 'https://cdn.example.com/completed-import.mp3', $2, $3)`, userID, completedOld, episodeID); err != nil {
		t.Fatal(err)
	}

	repository := NewMaintenanceRepository(pool)
	preview, err := repository.Retain(ctx, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview != (RetentionReport{CompletedJobs: 1, FailedJobs: 1, JobEvents: 2}) {
		t.Fatalf("preview=%+v", preview)
	}
	var jobsBefore int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE user_id = $1", userID).Scan(&jobsBefore); err != nil || jobsBefore != 5 {
		t.Fatalf("jobs before=%d err=%v", jobsBefore, err)
	}

	applied, err := repository.Retain(ctx, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied != preview {
		t.Fatalf("applied=%+v preview=%+v", applied, preview)
	}
	var jobs, events int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE user_id = $1", userID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_events AS event
        JOIN jobs AS job ON job.id = event.job_id WHERE job.user_id = $1`, userID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if jobs != 3 || events != 1 {
		t.Fatalf("jobs=%d events=%d", jobs, events)
	}
	status, err := db.New(pool).GetImportStatus(ctx, db.GetImportStatusParams{ImportID: importID(t, ctx, pool, userID, "expired-import"), UserID: userID})
	if err != nil || status.Status != "failed" || status.Stage != "expired" {
		t.Fatalf("expired import status=%+v err=%v", status, err)
	}
	status, err = db.New(pool).GetImportStatus(ctx, db.GetImportStatusParams{ImportID: importID(t, ctx, pool, userID, "completed-import"), UserID: userID})
	if err != nil || status.Status != "succeeded" || status.Stage != "completed" || status.EpisodeID != episodeID {
		t.Fatalf("completed import status=%+v err=%v", status, err)
	}
}

func insertRetentionJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID pgtype.UUID, status string, completedAt time.Time) pgtype.UUID {
	t.Helper()
	var jobID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO jobs
        (user_id, type, entity_type, entity_id, status, stage, completed_at)
        VALUES ($1, 'retention_terminal', 'retention_test', gen_random_uuid(), $2, $2, $3)
        RETURNING id`, userID, status, completedAt).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func importID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID pgtype.UUID, name string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM imports WHERE user_id = $1 AND submitted_url LIKE '%' || $2 || '%'", userID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
