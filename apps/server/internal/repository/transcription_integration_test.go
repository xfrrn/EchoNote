package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRetryQueuesOnlyFailedTranscriptionChunksAtTheirRecoveryPoint(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-transcription-retry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomUUID(t)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id = $1", userID)
	}()

	imports := NewImportRepository(pool)
	createdImport, err := imports.Create(ctx, userID, "https://cdn.example.com/retry-chunk.mp3")
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, createdImport.SubmittedUrl, &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "Retry one chunk",
		CanonicalURL: createdImport.SubmittedUrl, AudioURL: createdImport.SubmittedUrl,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewTranscriptionRepository(pool)
	run, err := repository.Create(ctx, userID, episodeID, "economy", RunConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'succeeded', stage = 'completed', completed_at = now(), updated_at = now()
		WHERE user_id = $1 AND entity_id = $2 AND type = 'download_audio'`, userID, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE transcription_runs
		SET status = 'failed', stage = 'failed', source_object_key = 'source', prepared_object_key = 'prepared.flac',
		    duration_ms = 20000, total_chunks = 4, completed_chunks = 1,
		    error_code = 'ASR_TASK_FAILED', error_message = 'one chunk failed', completed_at = now(), updated_at = now()
		WHERE id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE episodes SET transcription_status = 'failed' WHERE id = $1`, episodeID); err != nil {
		t.Fatal(err)
	}
	var completedID, failedID, submittedID, resultID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
		    transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
		    status, object_key, audio_hash, fingerprint, normalized_result, speaker_map, completed_at
		) VALUES ($1, 0, 0, 5000, 0, 6000, 'completed', 'chunk-0.flac', repeat('a', 64), repeat('b', 64),
		          '{"segments":[]}', '{"0":"speaker-001"}', now()) RETURNING id`, run.ID).Scan(&completedID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
		    transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
		    status, object_key, audio_hash, fingerprint, error_code, error_message
		) VALUES ($1, 1, 5000, 10000, 4000, 10000, 'failed', 'chunk-1.flac', repeat('c', 64), repeat('d', 64),
		          'ASR_TASK_FAILED', 'provider failed') RETURNING id`, run.ID).Scan(&failedID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
		    transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
		    status, object_key, audio_hash, fingerprint, external_task_id, error_code, error_message
		) VALUES ($1, 2, 10000, 15000, 9000, 16000, 'failed', 'chunk-2.flac', repeat('e', 64), repeat('f', 64),
		          'task-existing', 'ASR_POLL_FAILED', 'poll failed') RETURNING id`, run.ID).Scan(&submittedID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
		    transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
		    status, object_key, audio_hash, fingerprint, external_task_id, result_url, error_code, error_message
		) VALUES ($1, 3, 15000, 20000, 14000, 20000, 'failed', 'chunk-3.flac', repeat('1', 64), repeat('2', 64),
		          'task-completed', 'https://result.example.com/chunk-3.json', 'ASR_INGEST_FAILED', 'ingest failed') RETURNING id`, run.ID).Scan(&resultID); err != nil {
		t.Fatal(err)
	}

	retried, err := repository.Retry(ctx, userID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != "transcribing" || retried.CompletedChunks != 1 {
		t.Fatalf("retried=%+v", retried)
	}
	chunks, err := repository.Chunks(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chunks[0].ID != completedID || chunks[0].Status != "completed" || chunks[1].ID != failedID || chunks[1].Status != "ready" ||
		chunks[2].ID != submittedID || chunks[2].Status != "submitted" || chunks[3].ID != resultID || chunks[3].Status != "running" {
		t.Fatalf("chunks=%+v", chunks)
	}
	var submitQueued, pollQueued, ingestQueued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM jobs
		WHERE user_id = $1 AND type = 'submit_asr' AND entity_type = 'transcription_chunk'
		  AND entity_id = $2 AND status = 'queued'`, userID, failedID).Scan(&submitQueued); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE user_id = $1 AND type = 'poll_asr' AND entity_id = $2 AND status = 'queued'`, userID, submittedID).Scan(&pollQueued); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE user_id = $1 AND type = 'ingest_asr_result' AND entity_id = $2 AND status = 'queued'`, userID, resultID).Scan(&ingestQueued); err != nil {
		t.Fatal(err)
	}
	if submitQueued != 1 || pollQueued != 1 || ingestQueued != 1 {
		t.Fatalf("queued submit=%d poll=%d ingest=%d", submitQueued, pollQueued, ingestQueued)
	}

	if _, err := pool.Exec(ctx, `UPDATE transcription_runs SET status = 'failed', stage = 'failed', completed_at = now() WHERE id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Create(ctx, userID, episodeID, "economy", RunConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Retry(ctx, userID, run.ID); !errors.Is(err, ErrTranscriptionRunning) {
		t.Fatalf("retry while newer run active error=%v", err)
	}
}
