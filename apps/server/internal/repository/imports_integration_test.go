package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
)

func TestImportCreatesAndDeduplicatesEpisode(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-import-test")
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
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id = $1", userID)
	}()

	repository := NewImportRepository(pool)
	firstImport, err := repository.Create(ctx, userID, "https://podcasts.apple.com/show/id1?i=2")
	if err != nil || firstImport.Status != "queued" {
		t.Fatalf("first import status=%q err=%v", firstImport.Status, err)
	}
	secondImport, err := repository.Create(ctx, userID, "https://feeds.example.com/show.xml")
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	appleEpisode := &domain.ResolvedEpisode{
		SourceType: domain.SourceApple, ExternalID: "2", RSSGUID: "same-guid",
		PodcastTitle: "Show", PodcastAuthor: "Host", FeedURL: "https://feeds.example.com/show.xml",
		EpisodeTitle: "Episode", PublishedAt: &publishedAt, DurationMS: 60000,
		CanonicalURL: firstImport.SubmittedUrl, AudioURL: "https://cdn.example.com/episode.mp3",
	}
	rssEpisode := &domain.ResolvedEpisode{
		SourceType: domain.SourceRSS, RSSGUID: "same-guid",
		PodcastTitle: "Show", PodcastAuthor: "Host", FeedURL: "https://feeds.example.com/show.xml",
		EpisodeTitle: "Episode", PublishedAt: &publishedAt, DurationMS: 60000,
		CanonicalURL: "https://example.com/episode", AudioURL: "https://cdn.example.com/episode.mp3",
	}
	type saveResult struct {
		episodeID [16]byte
		err       error
	}
	results := make(chan saveResult, 2)
	go func() {
		id, saveErr := repository.SaveResolved(ctx, userID, firstImport.ID, firstImport.SubmittedUrl, appleEpisode)
		results <- saveResult{episodeID: id.Bytes, err: saveErr}
	}()
	go func() {
		id, saveErr := repository.SaveResolved(ctx, userID, secondImport.ID, secondImport.SubmittedUrl, rssEpisode)
		results <- saveResult{episodeID: id.Bytes, err: saveErr}
	}()
	firstResult, secondResult := <-results, <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent saves: first=%v second=%v", firstResult.err, secondResult.err)
	}
	if firstResult.episodeID != secondResult.episodeID {
		t.Fatalf("duplicate Episode IDs: first=%x second=%x", firstResult.episodeID, secondResult.episodeID)
	}

	var episodes, sources int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episodes WHERE user_id = $1", userID).Scan(&episodes); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM episode_sources WHERE user_id = $1", userID).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if episodes != 1 || sources != 2 {
		t.Fatalf("episodes=%d sources=%d, want 1 and 2", episodes, sources)
	}
}
