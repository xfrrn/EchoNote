package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeEmbeddingProvider struct {
	documentCalls int
	failQuery     bool
}

func (*fakeEmbeddingProvider) Model() string   { return "fake-search-v1" }
func (*fakeEmbeddingProvider) Dimensions() int { return 1024 }

func (provider *fakeEmbeddingProvider) Embed(_ context.Context, texts []string, inputType searchdomain.EmbeddingInputType) ([][]float32, error) {
	if inputType == searchdomain.EmbeddingQuery && provider.failQuery {
		return nil, errors.New("embedding unavailable")
	}
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector := make([]float32, provider.Dimensions())
		if strings.Contains(text, "融资") || strings.Contains(text, "资本扩张") {
			vector[0] = 1
		} else {
			vector[1] = 1
		}
		vectors[index] = vector
	}
	if inputType == searchdomain.EmbeddingDocument {
		provider.documentCalls++
	}
	return vectors, nil
}

func TestSearchIndexKeywordEmbeddingAndHybrid(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-search-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, otherUserID := randomServiceUUID(t), randomServiceUUID(t)
	defer ensureTestUsers(t, pool, userID, otherUserID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id IN ($1, $2)", userID, otherUserID)
	}()

	imports := repository.NewImportRepository(pool)
	audioURL := fmt.Sprintf("https://cdn.example.com/search-%x.mp3", userID.Bytes)
	createdImport, err := imports.Create(ctx, userID, audioURL)
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, audioURL, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "搜索验收节目",
		CanonicalURL: audioURL, AudioURL: audioURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := repository.NewNotesRepository(pool)
	note, err := notes.CreateForEpisode(ctx, userID, episodeID, randomServiceUUID(t), "我的独家观察与结论 microservice architecture", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	var runID, chunkID, transcriptID, speakerID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_runs (
			user_id, episode_id, profile, provider, model, status, stage,
			total_chunks, completed_chunks, completed_at
		) VALUES ($1, $2, 'economy', 'test', 'test', 'completed', 'completed', 1, 1, now())
		RETURNING id
	`, userID, episodeID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
			transcription_run_id, sequence, core_start_ms, core_end_ms,
			render_start_ms, render_end_ms, status, completed_at
		) VALUES ($1, 0, 0, 60000, 0, 60000, 'completed', now())
		RETURNING id
	`, runID).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_versions (user_id, episode_id, transcription_run_id, version, is_active)
		VALUES ($1, $2, $3, 1, true)
		RETURNING id
	`, userID, episodeID, runID).Scan(&transcriptID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_speakers (transcript_version_id, stable_key, display_name)
		VALUES ($1, 'global-1', '主讲人')
		RETURNING id
	`, transcriptID).Scan(&speakerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO transcript_segments (
			transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id
		) VALUES ($1, $2, 0, 1000, 5000, '公司完成新一轮融资，准备拓展海外市场', $3)
	`, transcriptID, speakerID, chunkID); err != nil {
		t.Fatal(err)
	}

	searchRepository := repository.NewSearchRepository(pool)
	provider := &fakeEmbeddingProvider{}
	build, err := searchRepository.BuildEpisode(ctx, userID, episodeID, provider.Model())
	if err != nil || build.Documents != 2 || build.Changed != 2 {
		t.Fatalf("build=%+v err=%v", build, err)
	}
	queue := repository.NewJobQueue(pool)
	embedHandler := NewSearchWorkflow(searchRepository, provider).Handlers()[repository.GenerateEmbeddingsJobType]
	for range 2 {
		job, found, err := queue.Claim(ctx, "search-test-worker", []string{repository.GenerateEmbeddingsJobType})
		if err != nil || !found {
			var jobs string
			_ = pool.QueryRow(ctx, "SELECT COALESCE(string_agg(type || ':' || status || ':' || (run_after - now())::text, ','), '') FROM jobs WHERE user_id = $1", userID).Scan(&jobs)
			t.Fatalf("claim found=%t err=%v jobs=%s", found, err, jobs)
		}
		if err := embedHandler(ctx, job); err != nil {
			t.Fatal(err)
		}
		if err := queue.Complete(ctx, job.ID, "search-test-worker"); err != nil {
			t.Fatal(err)
		}
	}
	if provider.documentCalls != 2 {
		t.Fatalf("document embedding calls=%d want=2", provider.documentCalls)
	}

	searchService := NewSearchService(searchRepository, provider)
	noteResults, err := searchService.Search(ctx, userID, "独家", "library", pgtype.UUID{}, 10)
	if err != nil || noteResults.Mode != "hybrid" || len(noteResults.Items) == 0 || noteResults.Items[0].DocumentType != "note" || noteResults.Items[0].SourceID != formatServiceUUID(note.Note.ID) {
		t.Fatalf("note results=%+v err=%v", noteResults, err)
	}
	provider.failQuery = true
	fallback, err := searchService.Search(ctx, userID, "独家", "library", pgtype.UUID{}, 10)
	provider.failQuery = false
	if err != nil || fallback.Mode != "keyword" || fallback.SemanticError == nil || len(fallback.Items) == 0 {
		t.Fatalf("fallback=%+v err=%v", fallback, err)
	}
	fuzzyResults, err := NewSearchService(searchRepository, nil).Search(ctx, userID, "microservce", "library", pgtype.UUID{}, 10)
	if err != nil || fuzzyResults.Mode != "keyword" || len(fuzzyResults.Items) == 0 || fuzzyResults.Items[0].DocumentType != "note" {
		t.Fatalf("fuzzy results=%+v err=%v", fuzzyResults, err)
	}
	semanticResults, err := searchService.Search(ctx, userID, "资本扩张", "episode", episodeID, 10)
	if err != nil || len(semanticResults.Items) == 0 || semanticResults.Items[0].DocumentType != "transcript" || semanticResults.Items[0].SpeakerName != "主讲人" || semanticResults.Items[0].StartMS == nil || *semanticResults.Items[0].StartMS != 1000 {
		t.Fatalf("semantic results=%+v err=%v", semanticResults, err)
	}
	isolated, err := searchService.Search(ctx, otherUserID, "独家", "library", pgtype.UUID{}, 10)
	if err != nil || len(isolated.Items) != 0 {
		t.Fatalf("isolated=%+v err=%v", isolated, err)
	}

	rebuilt, err := searchRepository.BuildEpisode(ctx, userID, episodeID, provider.Model())
	if err != nil || rebuilt.Changed != 0 {
		t.Fatalf("unchanged rebuild=%+v err=%v", rebuilt, err)
	}
	var embeddingJobs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE user_id = $1 AND type = 'generate_embeddings'", userID).Scan(&embeddingJobs); err != nil || embeddingJobs != 2 {
		t.Fatalf("embedding jobs=%d err=%v", embeddingJobs, err)
	}
	if _, err := notes.Update(ctx, userID, note.Note.ID, "更新后的独家结论"); err != nil {
		t.Fatal(err)
	}
	rebuilt, err = searchRepository.BuildEpisode(ctx, userID, episodeID, provider.Model())
	if err != nil || rebuilt.Changed != 1 {
		t.Fatalf("updated rebuild=%+v err=%v", rebuilt, err)
	}
	if queued, err := searchService.Reindex(ctx, userID, episodeID); err != nil || queued != 1 {
		t.Fatalf("reindex queued=%d err=%v", queued, err)
	}
}

func formatServiceUUID(id pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
