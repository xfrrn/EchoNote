package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Pinger interface {
	Ping(context.Context) error
}

type Server struct {
	database             Pinger
	imports              *repository.ImportRepository
	library              *repository.LibraryRepository
	notes                *repository.NotesRepository
	transcriptions       *repository.TranscriptionRepository
	searches             *service.SearchService
	ai                   *service.AIService
	transcriptionEnabled bool
	userID               pgtype.UUID
	logger               *slog.Logger
}

var _ ServerInterface = (*Server)(nil)

func NewRouter(
	database Pinger,
	imports *repository.ImportRepository,
	library *repository.LibraryRepository,
	notes *repository.NotesRepository,
	transcriptions *repository.TranscriptionRepository,
	searches *service.SearchService,
	ai *service.AIService,
	transcriptionEnabled bool,
	userID pgtype.UUID,
	logger *slog.Logger,
) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger, formatUUID(userID)))
	router.Use(recoverer(logger))
	return HandlerFromMux(&Server{
		database: database, imports: imports, library: library, notes: notes,
		transcriptions: transcriptions, searches: searches, ai: ai, transcriptionEnabled: transcriptionEnabled,
		userID: userID, logger: logger,
	}, router)
}

func (s *Server) CreateCapture(w http.ResponseWriter, r *http.Request) {
	var request CreateCaptureRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one capture")
		return
	}
	clientNoteID, content, err := parseNoteInput(request.ClientNoteId, request.Content, request.CreatedAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE", "client_note_id, content, and created_at are required")
		return
	}
	episodeID := valueOrEmpty(request.EpisodeId)
	episodeURL := valueOrEmpty(request.EpisodeUrl)
	if (episodeID == "") == (episodeURL == "") {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CAPTURE_TARGET", "provide exactly one of episode_id and episode_url")
		return
	}

	var result repository.CaptureResult
	if episodeID != "" {
		parsedEpisodeID, parseErr := parseUUID(episodeID)
		if parseErr != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episode_id must be a UUID")
			return
		}
		result, err = s.notes.CreateForEpisode(r.Context(), s.userID, parsedEpisodeID, clientNoteID, content, request.CreatedAt)
	} else {
		episodeURL, err = parseHTTPURL(episodeURL)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_URL", "episode_url must be an HTTP or HTTPS URL")
			return
		}
		result, err = s.notes.CaptureURL(r.Context(), s.userID, clientNoteID, episodeURL, content, request.CreatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "create capture", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	response := CaptureResponse{Note: noteResponse(result.Note)}
	if result.ImportID.Valid {
		importID := formatUUID(result.ImportID)
		response.ImportId = &importID
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, response)
}

func (s *Server) ListEpisodeNotes(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	rows, err := s.notes.List(r.Context(), s.userID, parsedEpisodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list notes", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "episode_id", episodeID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	items := make([]Note, len(rows))
	for index, row := range rows {
		items[index] = noteResponse(row)
	}
	writeJSON(w, http.StatusOK, NoteListResponse{Items: items})
}

func (s *Server) CreateEpisodeNote(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	var request CreateNoteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one note")
		return
	}
	clientNoteID, content, err := parseNoteInput(request.ClientNoteId, request.Content, request.CreatedAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE", "client_note_id, content, and created_at are required")
		return
	}
	result, err := s.notes.CreateForEpisode(r.Context(), s.userID, parsedEpisodeID, clientNoteID, content, request.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "create note", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "episode_id", episodeID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, noteResponse(result.Note))
}

func (s *Server) UpdateNote(w http.ResponseWriter, r *http.Request, noteID string) {
	parsedNoteID, err := parseUUID(noteID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE_ID", "noteId must be a UUID")
		return
	}
	var request UpdateNoteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain note content")
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE", "content is required")
		return
	}
	note, err := s.notes.Update(r.Context(), s.userID, parsedNoteID, request.Content)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "NOTE_NOT_FOUND", "note was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "update note", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "note_id", noteID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, noteResponse(note))
}

func (s *Server) DeleteNote(w http.ResponseWriter, r *http.Request, noteID string) {
	parsedNoteID, err := parseUUID(noteID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_NOTE_ID", "noteId must be a UUID")
		return
	}
	if err := s.notes.Delete(r.Context(), s.userID, parsedNoteID); errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "NOTE_NOT_FOUND", "note was not found")
		return
	} else if err != nil {
		s.logger.ErrorContext(r.Context(), "delete note", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "note_id", noteID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	s.logger.InfoContext(r.Context(), "note deleted", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "note_id", noteID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ListEpisodes(w http.ResponseWriter, r *http.Request, params ListEpisodesParams) {
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > math.MaxInt32 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit must be 1-100 and offset must be non-negative")
		return
	}
	rows, total, err := s.library.List(r.Context(), s.userID, int32(limit), int32(offset))
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list episodes", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	items := make([]EpisodeSummary, len(rows))
	for index, row := range rows {
		items[index] = episodeSummary(row)
	}
	writeJSON(w, http.StatusOK, EpisodeListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) GetEpisode(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	detail, err := s.library.Get(r.Context(), s.userID, parsedID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "get episode", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "episode_id", episodeID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, episodeDetail(detail))
}

func (s *Server) DeleteEpisode(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	if err := s.library.Delete(r.Context(), s.userID, parsedID); errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	} else if err != nil {
		s.logger.ErrorContext(r.Context(), "delete episode", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "episode_id", episodeID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	s.logger.InfoContext(r.Context(), "episode deleted", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "episode_id", episodeID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) CreateImport(w http.ResponseWriter, r *http.Request) {
	var request CreateImportRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one URL")
		return
	}
	parsedURL, err := parseHTTPURL(request.Url)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_URL", "url must be an HTTP or HTTPS URL")
		return
	}
	request.Url = parsedURL
	status, err := s.imports.Create(r.Context(), s.userID, request.Url)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "create import", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusAccepted, importResponse(status))
}

func (s *Server) GetImport(w http.ResponseWriter, r *http.Request, importID string) {
	parsedID, err := parseUUID(importID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_IMPORT_ID", "importId must be a UUID")
		return
	}
	status, err := s.imports.Get(r.Context(), s.userID, parsedID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "IMPORT_NOT_FOUND", "import was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "get import", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "import_id", importID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, importResponse(status))
}

func (s *Server) GetLiveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, StatusResponse{Status: StatusResponseStatusOk})
}

func (s *Server) GetReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.database.Ping(ctx); err != nil {
		s.logger.WarnContext(r.Context(), "readiness check failed",
			"request_id", middleware.GetReqID(r.Context()),
			"dependency", "postgresql",
			"error", err,
		)
		writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{
			Status:   ReadinessResponseStatusUnavailable,
			Database: Down,
		})
		return
	}
	writeJSON(w, http.StatusOK, ReadinessResponse{
		Status:   ReadinessResponseStatusOk,
		Database: Up,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestLogger(logger *slog.Logger, userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			requestID := middleware.GetReqID(r.Context())
			w.Header().Set("X-Request-ID", requestID)
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			logger.InfoContext(r.Context(), "http request",
				"request_id", requestID,
				"user_id", userID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.Status(),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func importResponse(status db.GetImportStatusRow) ImportResponse {
	response := ImportResponse{
		Id: formatUUID(status.ID), Url: status.SubmittedUrl, Status: ImportResponseStatus(status.Status),
		Stage: status.Stage, CreatedAt: status.CreatedAt.Time, UpdatedAt: status.UpdatedAt.Time,
	}
	if status.EpisodeID.Valid {
		episodeID := formatUUID(status.EpisodeID)
		response.EpisodeId = &episodeID
	}
	if status.ErrorCode != nil {
		message := "import failed"
		if status.ErrorMessage != nil && *status.ErrorMessage != "" {
			message = *status.ErrorMessage
		}
		response.Error = &ImportError{Code: *status.ErrorCode, Message: message}
	}
	return response
}

func episodeSummary(row db.ListLibraryEpisodesRow) EpisodeSummary {
	response := EpisodeSummary{
		Id: formatUUID(row.ID), Podcast: podcastSummary(
			row.PodcastID, row.PodcastTitle, row.PodcastAuthor, row.PodcastDescription, row.PodcastCoverUrl, row.PodcastFeedUrl,
		),
		Title: row.Title, DurationMs: row.DurationMs, CoverUrl: episodeCover(row.CoverUrl, row.PodcastCoverUrl),
		ResolveStatus: ResolveStatus(row.ResolveStatus), TranscriptionStatus: ProcessingStatus(row.TranscriptionStatus),
		AiStatus: ProcessingStatus(row.AiStatus), SourceCount: row.SourceCount, NoteCount: row.NoteCount,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.PublishedAt.Valid {
		publishedAt := row.PublishedAt.Time
		response.PublishedAt = &publishedAt
	}
	return response
}

func episodeDetail(detail repository.LibraryEpisodeDetail) EpisodeDetail {
	row := detail.Episode
	sources := make([]EpisodeSource, len(detail.Sources))
	for index, source := range detail.Sources {
		sources[index] = EpisodeSource{
			Id: formatUUID(source.ID), SourceType: SourceType(source.SourceType), ExternalId: source.ExternalID,
			SourceUrl: source.SourceUrl, CanonicalUrl: source.CanonicalUrl, RssGuid: source.RssGuid,
			CreatedAt: source.CreatedAt.Time,
		}
	}
	response := EpisodeDetail{
		Id: formatUUID(row.ID), Podcast: podcastSummary(
			row.PodcastID, row.PodcastTitle, row.PodcastAuthor, row.PodcastDescription, row.PodcastCoverUrl, row.PodcastFeedUrl,
		),
		Title: row.Title, Description: row.Description, DurationMs: row.DurationMs,
		CoverUrl: episodeCover(row.CoverUrl, row.PodcastCoverUrl), ResolveStatus: ResolveStatus(row.ResolveStatus),
		TranscriptionStatus: ProcessingStatus(row.TranscriptionStatus), AiStatus: ProcessingStatus(row.AiStatus),
		SourceCount: row.SourceCount, Sources: sources, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.PublishedAt.Valid {
		publishedAt := row.PublishedAt.Time
		response.PublishedAt = &publishedAt
	}
	return response
}

func podcastSummary(
	id pgtype.UUID,
	title, author, description, coverURL, feedURL *string,
) *PodcastSummary {
	if !id.Valid {
		return nil
	}
	return &PodcastSummary{
		Id: formatUUID(id), Title: valueOrEmpty(title), Author: valueOrEmpty(author),
		Description: valueOrEmpty(description), CoverUrl: valueOrEmpty(coverURL), FeedUrl: feedURL,
	}
}

func episodeCover(episodeCoverURL string, podcastCoverURL *string) string {
	if episodeCoverURL != "" || podcastCoverURL == nil {
		return episodeCoverURL
	}
	return *podcastCoverURL
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func noteResponse(row db.Note) Note {
	response := Note{
		Id: formatUUID(row.ID), EpisodeId: formatUUID(row.EpisodeID), ClientNoteId: formatUUID(row.ClientNoteID),
		Content: row.Content, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.DeletedAt.Valid {
		deletedAt := row.DeletedAt.Time
		response.DeletedAt = &deletedAt
	}
	return response
}

func parseNoteInput(clientNoteID, content string, createdAt time.Time) (pgtype.UUID, string, error) {
	parsedClientNoteID, err := parseUUID(strings.TrimSpace(clientNoteID))
	content = strings.TrimSpace(content)
	if err != nil || content == "" || createdAt.IsZero() {
		return pgtype.UUID{}, "", errors.New("invalid note input")
	}
	return parsedClientNoteID, content, nil
}

func parseHTTPURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || len(value) > 4096 {
		return "", errors.New("invalid HTTP URL")
	}
	return value, nil
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Code: code, Message: message})
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	bytes := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	return id, err
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "http panic",
						"request_id", middleware.GetReqID(r.Context()),
						"panic", fmt.Sprint(recovered),
						"stack", string(debug.Stack()),
					)
					writeJSON(w, http.StatusInternalServerError, map[string]string{
						"code":    "INTERNAL_ERROR",
						"message": "internal server error",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
