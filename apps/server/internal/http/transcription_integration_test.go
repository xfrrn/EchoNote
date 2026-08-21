package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	transcriptiondomain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestTranscriptionHTTPFlow(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-transcription-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomHTTPUUID(t)
	defer ensureTestUsers(t, pool, userID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id = $1", userID)
	}()

	imports := repository.NewImportRepository(pool)
	createdImport, err := imports.Create(ctx, userID, "https://cdn.example.com/http-transcript.mp3")
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, createdImport.SubmittedUrl, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "HTTP Transcript",
		CanonicalURL: createdImport.SubmittedUrl, AudioURL: createdImport.SubmittedUrl,
	})
	if err != nil {
		t.Fatal(err)
	}
	transcriptions := repository.NewTranscriptionRepository(pool)
	router := NewRouter(
		pool, imports, repository.NewLibraryRepository(pool), repository.NewNotesRepository(pool),
		transcriptions, nil, nil, nil, nil, true, developmentAuth(userID), slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	path := "/api/v1/episodes/" + formatUUID(episodeID) + "/transcriptions"
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"profile":"quality","language_hint":"en","speaker_count":2}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var run TranscriptionRun
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != TranscriptionRunStatusQueued || run.Model != "fun-asr" {
		t.Fatalf("run=%+v", run)
	}

	request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"profile":"economy"}`))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
	}

	runID, err := parseUUID(run.Id)
	if err != nil {
		t.Fatal(err)
	}
	var chunkID pgtype.UUID
	if _, err := pool.Exec(ctx, `
		UPDATE transcription_runs
		SET status = 'merging', stage = 'merging_transcript', total_chunks = 1, completed_chunks = 1,
		    started_at = now(), updated_at = now()
		WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
		    transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
		    status, normalized_result, speaker_map, completed_at
		) VALUES ($1, 0, 0, 10000, 0, 10000, 'completed', '{"segments":[]}', '{"0":"speaker-001","1":"speaker-002"}', now())
		RETURNING id`, runID).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	version, err := transcriptions.ActivateTranscript(ctx, runID,
		[]transcriptiondomain.Speaker{
			{StableKey: "speaker-001", DisplayName: "Speaker A"},
			{StableKey: "speaker-002", DisplayName: "Speaker B"},
		},
		[]transcriptiondomain.MergedSegment{
			{SpeakerKey: "speaker-001", Sequence: 0, StartMS: 0, EndMS: 4000, Text: "hello", SourceChunkID: chunkID.String()},
			{SpeakerKey: "speaker-002", Sequence: 1, StartMS: 5000, EndMS: 9000, Text: "world", SourceChunkID: chunkID.String()},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/transcriptions/"+run.Id, nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"completed"`) {
		t.Fatalf("get run status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/episodes/"+formatUUID(episodeID)+"/transcript", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var transcript Transcript
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &transcript) != nil || len(transcript.Speakers) != 2 {
		t.Fatalf("transcript status=%d body=%s", response.Code, response.Body.String())
	}
	if transcript.Id != formatUUID(version.ID) {
		t.Fatalf("transcript=%+v", transcript)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/transcripts/"+transcript.Id+"/segments", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var list TranscriptSegmentList
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &list) != nil || list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("segments status=%d body=%s", response.Code, response.Body.String())
	}

	target, source := transcript.Speakers[0], transcript.Speakers[1]
	request = httptest.NewRequest(http.MethodPatch,
		"/api/v1/transcripts/"+transcript.Id+"/speakers/"+target.Id,
		strings.NewReader(`{"display_name":"Host","role":"host"}`),
	)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"display_name":"Host"`) {
		t.Fatalf("rename status=%d body=%s", response.Code, response.Body.String())
	}

	mergeBody := fmt.Sprintf(`{"source_speaker_id":%q,"target_speaker_id":%q}`, source.Id, target.Id)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/transcripts/"+transcript.Id+"/speakers/merge", strings.NewReader(mergeBody))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("merge status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/transcripts/"+transcript.Id+"/segments", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if json.Unmarshal(response.Body.Bytes(), &list) != nil {
		t.Fatal(response.Body.String())
	}
	for _, segment := range list.Items {
		if segment.SpeakerId != target.Id {
			t.Fatalf("segment speaker=%s target=%s", segment.SpeakerId, target.Id)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/transcriptions/"+run.Id+"/events", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(response.Body.String(), "event: completed") {
		t.Fatalf("events status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/transcriptions/"+run.Id+"/retry", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("retry completed status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"profile":"economy"}`))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var canceledRun TranscriptionRun
	if response.Code != http.StatusAccepted || json.Unmarshal(response.Body.Bytes(), &canceledRun) != nil {
		t.Fatalf("second create status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/transcriptions/"+canceledRun.Id+"/cancel", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &canceledRun) != nil || canceledRun.Status != TranscriptionRunStatusCanceled {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/episodes/"+formatUUID(episodeID), nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete transcribed episode status=%d body=%s", response.Code, response.Body.String())
	}
}
