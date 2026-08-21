package httpapi

import (
	"context"
	"crypto/rand"
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
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNotesHTTPFlow(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-notes-http-test")
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
	createdImport, err := imports.Create(ctx, userID, "https://cdn.example.com/http-note.mp3")
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, createdImport.SubmittedUrl, &domain.ResolvedEpisode{
		SourceType: domain.SourceDirectAudio, EpisodeTitle: "HTTP Notes Episode",
		CanonicalURL: createdImport.SubmittedUrl, AudioURL: createdImport.SubmittedUrl,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		pool,
		imports,
		repository.NewLibraryRepository(pool),
		repository.NewNotesRepository(pool),
		nil,
		nil,
		nil,
		nil,
		nil,
		false,
		developmentAuth(userID),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	clientNoteID := formatUUID(randomHTTPUUID(t))
	requestBody := fmt.Sprintf(
		`{"client_note_id":%q,"episode_id":%q,"content":"HTTP 离线笔记","created_at":"2026-08-20T19:32:00+08:00"}`,
		clientNoteID,
		formatUUID(episodeID),
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/captures", strings.NewReader(requestBody))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var capture CaptureResponse
	if err := json.Unmarshal(response.Body.Bytes(), &capture); err != nil {
		t.Fatal(err)
	}
	if capture.Note.ClientNoteId != clientNoteID || capture.Note.Content != "HTTP 离线笔记" || capture.ImportId != nil {
		t.Fatalf("capture=%+v", capture)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/captures", strings.NewReader(requestBody))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/episodes/"+formatUUID(episodeID)+"/notes", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var list NoteListResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/notes/"+capture.Note.Id, strings.NewReader(`{"content":"已更新"}`))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var updated Note
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &updated) != nil || updated.Content != "已更新" {
		t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/episodes", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var episodes EpisodeListResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &episodes) != nil || len(episodes.Items) != 1 || episodes.Items[0].NoteCount != 1 {
		t.Fatalf("library status=%d body=%s", response.Code, response.Body.String())
	}

	for range 2 {
		request = httptest.NewRequest(http.MethodDelete, "/api/v1/notes/"+capture.Note.Id, nil)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
		}
	}

	urlClientNoteID := formatUUID(randomHTTPUUID(t))
	requestBody = fmt.Sprintf(
		`{"client_note_id":%q,"episode_url":"https://cdn.example.com/capture.mp3","content":"先记后导入","created_at":"2026-08-20T20:00:00+08:00"}`,
		urlClientNoteID,
	)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/captures", strings.NewReader(requestBody))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &capture) != nil || capture.ImportId == nil {
		t.Fatalf("URL capture status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+*capture.ImportId, nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var importStatus ImportResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &importStatus) != nil || importStatus.Status != "queued" || importStatus.EpisodeId == nil {
		t.Fatalf("capture import status=%d body=%s", response.Code, response.Body.String())
	}
}

func randomHTTPUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	id := pgtype.UUID{Valid: true}
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		t.Fatal(err)
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	return id
}
