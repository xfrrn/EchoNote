package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type pingerFunc func(context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error {
	return f(ctx)
}

func testOwnerID(userID pgtype.UUID) pgtype.UUID {
	if !userID.Valid {
		userID = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	}
	return userID
}

func TestHealthEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name       string
		path       string
		pingError  error
		wantStatus int
		wantBody   string
	}{
		{name: "live", path: "/healthz", wantStatus: http.StatusOK, wantBody: `"status":"ok"`},
		{name: "ready", path: "/readyz", wantStatus: http.StatusOK, wantBody: `"database":"up"`},
		{name: "database down", path: "/readyz", pingError: errors.New("offline"), wantStatus: http.StatusServiceUnavailable, wantBody: `"database":"down"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter(pingerFunc(func(context.Context) error { return test.pingError }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", response.Body.String(), test.wantBody)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("X-Request-ID is missing")
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
			}
		})
	}
}

func TestImportRejectsInvalidInputBeforeDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/imports", body: `{"url":"file:///tmp/audio.mp3"}`},
		{method: http.MethodPost, path: "/api/v1/imports", body: `{"url":"https://example.com","extra":true}`},
		{method: http.MethodGet, path: "/api/v1/imports/not-a-uuid"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestLibraryRejectsInvalidParametersBeforeDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/episodes?limit=0"},
		{method: http.MethodGet, path: "/api/v1/episodes?limit=101"},
		{method: http.MethodGet, path: "/api/v1/episodes?offset=-1"},
		{method: http.MethodGet, path: "/api/v1/episodes/not-a-uuid"},
		{method: http.MethodDelete, path: "/api/v1/episodes/not-a-uuid"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestNotesRejectInvalidInputBeforeDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
	const (
		id        = "11111111-1111-4111-8111-111111111111"
		otherID   = "22222222-2222-4222-8222-222222222222"
		createdAt = "2026-08-20T19:32:00+08:00"
	)
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/captures", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/captures", body: `{"client_note_id":"` + id + `","content":"note","created_at":"` + createdAt + `"}`},
		{method: http.MethodPost, path: "/api/v1/captures", body: `{"client_note_id":"` + id + `","episode_id":"` + otherID + `","episode_url":"https://example.com/feed","content":"note","created_at":"` + createdAt + `"}`},
		{method: http.MethodPost, path: "/api/v1/captures", body: `{"client_note_id":"` + id + `","episode_id":"bad","content":"note","created_at":"` + createdAt + `"}`},
		{method: http.MethodPost, path: "/api/v1/captures", body: `{"client_note_id":"` + id + `","episode_url":"file:///tmp/audio.mp3","content":"note","created_at":"` + createdAt + `"}`},
		{method: http.MethodGet, path: "/api/v1/episodes/not-a-uuid/notes"},
		{method: http.MethodPost, path: "/api/v1/episodes/" + otherID + "/notes", body: `{"client_note_id":"` + id + `","content":" ","created_at":"` + createdAt + `"}`},
		{method: http.MethodPatch, path: "/api/v1/notes/not-a-uuid", body: `{"content":"note"}`},
		{method: http.MethodPatch, path: "/api/v1/notes/" + otherID, body: `{"content":" "}`},
		{method: http.MethodDelete, path: "/api/v1/notes/not-a-uuid"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestTranscriptionRejectsInvalidInputBeforeDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
	const (
		id      = "11111111-1111-4111-8111-111111111111"
		otherID = "22222222-2222-4222-8222-222222222222"
	)
	tests := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/episodes/not-a-uuid/transcriptions", `{"profile":"economy"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/episodes/" + id + "/transcriptions", `{"profile":"fast"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/episodes/" + id + "/transcriptions", `{"profile":"economy","language_hint":"../../"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/episodes/" + id + "/transcriptions", `{"profile":"economy"}`, http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/transcriptions/not-a-uuid", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/transcriptions/not-a-uuid/events", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/transcriptions/not-a-uuid/retry", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/transcriptions/not-a-uuid/cancel", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/episodes/not-a-uuid/transcript", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/transcripts/not-a-uuid/segments", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/transcripts/" + id + "/segments?limit=0", "", http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/transcripts/" + id + "/speakers/not-a-uuid", `{"display_name":"Host"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/transcripts/" + id + "/speakers/merge", `{"source_speaker_id":"` + otherID + `","target_speaker_id":"` + otherID + `"}`, http.StatusBadRequest},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s %s status=%d want=%d body=%s", test.method, test.path, response.Code, test.want, response.Body.String())
		}
	}
}

func TestSearchRejectsInvalidInputBeforeDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, nil, nil, nil, nil, nil, false, testOwnerID(pgtype.UUID{}), logger)
	const id = "11111111-1111-4111-8111-111111111111"
	tests := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/search?q=a", ""},
		{http.MethodGet, "/api/v1/search?q=valid&scope=episode", ""},
		{http.MethodGet, "/api/v1/search?q=valid&scope=library&episode_id=" + id, ""},
		{http.MethodPost, "/api/v1/search/reindex", `{"scope":"episode"}`},
		{http.MethodPost, "/api/v1/search/reindex", `{"scope":"library","episode_id":"` + id + `"}`},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}
