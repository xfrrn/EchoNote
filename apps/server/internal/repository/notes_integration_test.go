package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/jackc/pgx/v5"
)

func TestNotesLifecycleAndCaptureIdempotency(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-notes-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, otherUserID := randomUUID(t), randomUUID(t)
	defer ensureTestUsers(t, pool, userID, otherUserID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id IN ($1, $2)", userID, otherUserID)
	}()

	imports := NewImportRepository(pool)
	baseImport, err := imports.Create(ctx, userID, "https://podcasts.apple.com/show/id1?i=2")
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	baseEpisodeID, err := imports.SaveResolved(ctx, userID, baseImport.ID, baseImport.SubmittedUrl, &domain.ResolvedEpisode{
		SourceType: domain.SourceApple, ExternalID: "2", RSSGUID: "notes-guid",
		PodcastTitle: "Notes Show", FeedURL: "https://feeds.example.com/notes.xml",
		EpisodeTitle: "Notes Episode", PublishedAt: &publishedAt, DurationMS: 60000,
		CanonicalURL: baseImport.SubmittedUrl, AudioURL: "https://cdn.example.com/notes.mp3",
	})
	if err != nil {
		t.Fatal(err)
	}

	notes := NewNotesRepository(pool)
	clientNoteID := randomUUID(t)
	createdAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	type createResult struct {
		result CaptureResult
		err    error
	}
	results := make(chan createResult, 2)
	for range 2 {
		go func() {
			result, createErr := notes.CreateForEpisode(ctx, userID, baseEpisodeID, clientNoteID, "  离线笔记  ", createdAt)
			results <- createResult{result: result, err: createErr}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.result.Note.ID != second.result.Note.ID {
		t.Fatalf("idempotent create first=%+v second=%+v", first, second)
	}
	if first.result.Created == second.result.Created || first.result.Note.Content != "离线笔记" {
		t.Fatalf("created flags/content first=%+v second=%+v", first.result, second.result)
	}
	if _, err := notes.CreateForEpisode(ctx, otherUserID, baseEpisodeID, randomUUID(t), "越权", createdAt); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user create err=%v", err)
	}
	if _, err := notes.List(ctx, otherUserID, baseEpisodeID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user list err=%v", err)
	}

	mergeURL := "https://feeds.example.com/capture.xml"
	mergeCapture, err := notes.CaptureURL(ctx, userID, randomUUID(t), mergeURL, "合并到已有单集", createdAt.Add(time.Hour))
	if err != nil || !mergeCapture.Created || !mergeCapture.ImportID.Valid {
		t.Fatalf("merge capture=%+v err=%v", mergeCapture, err)
	}
	mergeReplay, err := notes.CaptureURL(ctx, userID, mergeCapture.Note.ClientNoteID, mergeURL, "合并到已有单集", createdAt.Add(time.Hour))
	if err != nil || mergeReplay.Created || mergeReplay.Note.ID != mergeCapture.Note.ID || mergeReplay.ImportID != mergeCapture.ImportID {
		t.Fatalf("merge replay=%+v err=%v", mergeReplay, err)
	}
	mergedEpisodeID, err := imports.SaveResolved(ctx, userID, mergeCapture.ImportID, mergeURL, &domain.ResolvedEpisode{
		SourceType: domain.SourceRSS, RSSGUID: "notes-guid", PodcastTitle: "Notes Show",
		FeedURL: "https://feeds.example.com/notes.xml", EpisodeTitle: "Notes Episode",
		PublishedAt: &publishedAt, DurationMS: 60000, CanonicalURL: mergeURL,
		AudioURL: "https://cdn.example.com/notes.mp3",
	})
	if err != nil || mergedEpisodeID != baseEpisodeID {
		t.Fatalf("merged episode=%x want=%x err=%v", mergedEpisodeID.Bytes, baseEpisodeID.Bytes, err)
	}
	mergeReplay, err = notes.CaptureURL(ctx, userID, mergeCapture.Note.ClientNoteID, mergeURL, "合并到已有单集", createdAt.Add(time.Hour))
	if err != nil || mergeReplay.Note.EpisodeID != baseEpisodeID || mergeReplay.ImportID != mergeCapture.ImportID {
		t.Fatalf("resolved replay=%+v err=%v", mergeReplay, err)
	}
	var pendingCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episodes WHERE id = $1", mergeCapture.Note.EpisodeID).Scan(&pendingCount); err != nil || pendingCount != 0 {
		t.Fatalf("merged pending count=%d err=%v", pendingCount, err)
	}

	directURL := "https://cdn.example.com/captured-direct.mp3"
	directCapture, err := notes.CaptureURL(ctx, userID, randomUUID(t), directURL, "保留在 Pending Episode", createdAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	directEpisodeID, err := imports.SaveResolved(ctx, userID, directCapture.ImportID, directURL, &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "Captured Direct",
		CanonicalURL: directURL, AudioURL: directURL,
	})
	if err != nil || directEpisodeID != directCapture.Note.EpisodeID {
		t.Fatalf("direct episode=%x pending=%x err=%v", directEpisodeID.Bytes, directCapture.Note.EpisodeID.Bytes, err)
	}
	var directTitle, directStatus string
	if err := pool.QueryRow(ctx, "SELECT title, resolve_status FROM episodes WHERE id = $1", directEpisodeID).Scan(&directTitle, &directStatus); err != nil || directTitle != "Captured Direct" || directStatus != "completed" {
		t.Fatalf("direct title=%q status=%q err=%v", directTitle, directStatus, err)
	}

	library := NewLibraryRepository(pool)
	canceledURL := "https://cdn.example.com/canceled-capture.mp3"
	canceledCapture, err := notes.CaptureURL(ctx, userID, randomUUID(t), canceledURL, "删除待解析记录", createdAt.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := library.Delete(ctx, userID, canceledCapture.Note.EpisodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.SaveResolved(ctx, userID, canceledCapture.ImportID, canceledURL, &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "Must Not Return",
		CanonicalURL: canceledURL, AudioURL: canceledURL,
	}); !errors.Is(err, ErrImportCanceled) {
		t.Fatalf("deleted capture resolve err=%v", err)
	}
	var canceledStatus string
	var canceledDetached bool
	if err := pool.QueryRow(ctx, `
		SELECT job.status, import_record.episode_id IS NULL
		FROM imports AS import_record
		JOIN jobs AS job ON job.id = import_record.job_id
		WHERE import_record.id = $1
	`, canceledCapture.ImportID).Scan(&canceledStatus, &canceledDetached); err != nil || canceledStatus != "canceled" || !canceledDetached {
		t.Fatalf("canceled status=%q detached=%t err=%v", canceledStatus, canceledDetached, err)
	}

	baseNotes, err := notes.List(ctx, userID, baseEpisodeID)
	if err != nil || len(baseNotes) != 2 || baseNotes[0].ID != mergeCapture.Note.ID {
		t.Fatalf("base notes=%+v err=%v", baseNotes, err)
	}
	updated, err := notes.Update(ctx, userID, first.result.Note.ID, "更新后的笔记")
	if err != nil || updated.Content != "更新后的笔记" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err := notes.Update(ctx, otherUserID, updated.ID, "越权更新"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user update err=%v", err)
	}
	if err := notes.Delete(ctx, userID, updated.ID); err != nil {
		t.Fatal(err)
	}
	if err := notes.Delete(ctx, userID, updated.ID); err != nil {
		t.Fatalf("idempotent delete err=%v", err)
	}
	deletedReplay, err := notes.CreateForEpisode(ctx, userID, baseEpisodeID, clientNoteID, "不会复活", createdAt)
	if err != nil || deletedReplay.Created || !deletedReplay.Note.DeletedAt.Valid {
		t.Fatalf("deleted replay=%+v err=%v", deletedReplay, err)
	}
	if err := notes.Delete(ctx, otherUserID, mergeCapture.Note.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user delete err=%v", err)
	}

	items, total, err := library.List(ctx, userID, 10, 0)
	if err != nil || total != 2 {
		t.Fatalf("library total=%d err=%v", total, err)
	}
	noteCounts := map[[16]byte]int64{}
	for _, item := range items {
		noteCounts[item.ID.Bytes] = item.NoteCount
	}
	if noteCounts[baseEpisodeID.Bytes] != 1 || noteCounts[directEpisodeID.Bytes] != 1 {
		t.Fatalf("note counts=%v", noteCounts)
	}

	if err := library.Delete(ctx, userID, baseEpisodeID); err != nil {
		t.Fatal(err)
	}
	if err := library.Delete(ctx, userID, directEpisodeID); err != nil {
		t.Fatal(err)
	}
	var remainingNotes, detachedImports int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notes WHERE user_id = $1", userID).Scan(&remainingNotes); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM imports WHERE user_id = $1 AND episode_id IS NULL", userID).Scan(&detachedImports); err != nil {
		t.Fatal(err)
	}
	if remainingNotes != 0 || detachedImports != 4 {
		t.Fatalf("remaining notes=%d detached imports=%d", remainingNotes, detachedImports)
	}
}
