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

func TestLibraryLifecycle(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-library-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, otherUserID := randomUUID(t), randomUUID(t)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id IN ($1, $2)", userID, otherUserID)
	}()

	imports := NewImportRepository(pool)
	publishedAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	podcastImport, err := imports.Create(ctx, userID, "https://podcasts.apple.com/show/id1?i=2")
	if err != nil {
		t.Fatal(err)
	}
	podcastEpisodeID, err := imports.SaveResolved(ctx, userID, podcastImport.ID, podcastImport.SubmittedUrl, &domain.ResolvedEpisode{
		SourceType: domain.SourceApple, ExternalID: "2", RSSGUID: "library-guid",
		PodcastTitle: "Library Show", PodcastAuthor: "Host", PodcastDescription: "Show description",
		PodcastCoverURL: "https://cdn.example.com/cover.jpg", FeedURL: "https://feeds.example.com/show.xml",
		EpisodeTitle: "Library Episode", Description: "Episode description", PublishedAt: &publishedAt,
		DurationMS: 60000, CanonicalURL: podcastImport.SubmittedUrl, AudioURL: "https://cdn.example.com/episode.mp3",
	})
	if err != nil {
		t.Fatal(err)
	}

	directImport, err := imports.Create(ctx, userID, "https://cdn.example.com/direct.mp3")
	if err != nil {
		t.Fatal(err)
	}
	directEpisodeID, err := imports.SaveResolved(ctx, userID, directImport.ID, directImport.SubmittedUrl, &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "Direct Episode",
		CanonicalURL: directImport.SubmittedUrl, AudioURL: directImport.SubmittedUrl,
	})
	if err != nil {
		t.Fatal(err)
	}

	library := NewLibraryRepository(pool)
	items, total, err := library.List(ctx, userID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 1 || items[0].ID != directEpisodeID || items[0].PodcastID.Valid ||
		items[0].ResolveStatus != "completed" || items[0].TranscriptionStatus != "waiting" || items[0].AiStatus != "waiting" {
		t.Fatalf("first page total=%d items=%+v", total, items)
	}
	secondPage, _, err := library.List(ctx, userID, 1, 1)
	if err != nil || len(secondPage) != 1 || secondPage[0].ID != podcastEpisodeID {
		t.Fatalf("second page=%+v err=%v", secondPage, err)
	}
	if otherItems, otherTotal, err := library.List(ctx, otherUserID, 10, 0); err != nil || otherTotal != 0 || len(otherItems) != 0 {
		t.Fatalf("other user items=%+v total=%d err=%v", otherItems, otherTotal, err)
	}

	detail, err := library.Get(ctx, userID, podcastEpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Episode.PodcastID.Valid || detail.Episode.SourceCount != 1 || len(detail.Sources) != 1 || detail.Sources[0].SourceType != domain.SourceApple {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if _, err := library.Get(ctx, otherUserID, podcastEpisodeID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user get err=%v", err)
	}
	if err := library.Delete(ctx, otherUserID, podcastEpisodeID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-user delete err=%v", err)
	}
	if err := library.Delete(ctx, userID, podcastEpisodeID); err != nil {
		t.Fatal(err)
	}
	if err := library.Delete(ctx, userID, directEpisodeID); err != nil {
		t.Fatal(err)
	}

	var episodes, sources, identities, podcasts, detachedImports int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episodes WHERE user_id = $1", userID).Scan(&episodes); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episode_sources WHERE user_id = $1", userID).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episode_identity_keys WHERE user_id = $1", userID).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM podcasts WHERE user_id = $1", userID).Scan(&podcasts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM imports WHERE user_id = $1 AND episode_id IS NULL", userID).Scan(&detachedImports); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 || sources != 0 || identities != 0 || podcasts != 0 || detachedImports != 2 {
		t.Fatalf("episodes=%d sources=%d identities=%d podcasts=%d detachedImports=%d", episodes, sources, identities, podcasts, detachedImports)
	}
}
