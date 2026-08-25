package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/database/db"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
)

func TestEpisodeExportHTTP(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-export-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, otherUserID := randomHTTPUUID(t), randomHTTPUUID(t)
	defer ensureTestUsers(t, pool, userID, otherUserID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id IN ($1, $2)", userID, otherUserID)
	}()

	imports := repository.NewImportRepository(pool)
	audioURL := fmt.Sprintf("https://cdn.example.com/export-%x.mp3", userID.Bytes)
	createdImport, err := imports.Create(ctx, userID, audioURL)
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 8, 20, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, audioURL, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "Export: 验收节目", PublishedAt: &published,
		CanonicalURL: audioURL, AudioURL: audioURL, DurationMS: 61_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := repository.NewNotesRepository(pool)
	if _, err := notes.CreateForEpisode(ctx, userID, episodeID, randomHTTPUUID(t), "我的导出笔记", published); err != nil {
		t.Fatal(err)
	}
	speakerID, segmentID := createAITranscript(t, ctx, pool, userID, episodeID)
	version, err := db.New(pool).GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	artifact := aidomain.ArtifactResult{
		OneSentenceSummary: "一句话导出总结", KeyPoints: []string{"导出核心观点"},
		SpeakerViews: []aidomain.SpeakerView{{SpeakerID: formatUUID(speakerID), SpeakerName: "主讲人", Points: []string{"人物观点"}}},
		WorthReviewing: []aidomain.WorthReviewing{{
			TranscriptSegmentID: formatUUID(segmentID), SpeakerID: formatUUID(speakerID), SpeakerName: "主讲人",
			StartMS: 1_000, EndMS: 5_000, Quote: "公司完成新一轮融资，准备拓展海外市场", Reason: "核心事实",
		}},
		NoteConnections: []aidomain.NoteConnection{},
	}
	rawArtifact, _ := json.Marshal(artifact)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_artifacts (
			user_id, episode_id, transcript_version_id, artifact_type, model, prompt_version,
			notes_revision, input_hash, status, result, search_text, completed_at
		) VALUES ($1, $2, $3, 'episode_summary', 'test', 'test-v1', $4, $5, 'ready', $6, 'search', now())
	`, userID, episodeID, version.ID, strings.Repeat("a", 64), strings.Repeat("b", 64), rawArtifact); err != nil {
		t.Fatal(err)
	}

	exportService := service.NewExportService(repository.NewExportRepository(pool))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pool, nil, nil, nil, nil, nil, nil, exportService, false, userID, logger)
	path := "/api/v1/episodes/" + formatUUID(episodeID) + "/exports"
	body := `{"mode":"organized_note","include_user_notes":true,"include_summary":true,"include_key_points":true,"include_worth_reviewing":true,"include_transcript":true,"transcript_segment_ids":["` + formatUUID(segmentID) + `"]}`
	response := serveAIRequest(router, http.MethodPost, path, body)
	var result ExportContent
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("organized export status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{"【一句话总结】", "【核心观点】", "【值得回顾】", "【我的笔记】", "【Transcript 节选】"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("export omitted %q: %s", expected, result.Text)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(result.Markdown, "# Export: 验收节目") || result.SuggestedFilename != "Export- 验收节目.md" {
		t.Fatalf("headers=%v result=%+v", response.Header(), result)
	}

	response = serveAIRequest(router, http.MethodPost, path, `{"mode":"selected_transcript","transcript_segment_ids":["`+formatUUID(randomHTTPUUID(t))+`"]}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("foreign segment status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, path, `{"mode":"full_transcript"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "完整 Transcript") {
		t.Fatalf("full transcript status=%d body=%s", response.Code, response.Body.String())
	}

	if _, err := pool.Exec(ctx, "UPDATE ai_artifacts SET status = 'stale' WHERE user_id = $1 AND episode_id = $2", userID, episodeID); err != nil {
		t.Fatal(err)
	}
	response = serveAIRequest(router, http.MethodPost, path, `{"mode":"organized_note"}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "AI_ARTIFACT_NOT_READY") {
		t.Fatalf("stale artifact status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, path, `{"mode":"notes_only"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "我的导出笔记") {
		t.Fatalf("notes export status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, path, `{"mode":"organized_note","include_user_notes":false,"include_summary":false,"include_key_points":false,"include_worth_reviewing":false}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty organized export status=%d body=%s", response.Code, response.Body.String())
	}

	otherRouter := NewRouter(pool, nil, nil, nil, nil, nil, nil, exportService, false, otherUserID, logger)
	response = serveAIRequest(otherRouter, http.MethodPost, path, `{"mode":"notes_only"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("export isolation status=%d body=%s", response.Code, response.Body.String())
	}
}
