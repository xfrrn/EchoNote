package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Pinger interface {
	Ping(context.Context) error
}

type Server struct {
	database Pinger
	imports  *repository.ImportRepository
	userID   pgtype.UUID
	logger   *slog.Logger
}

var _ ServerInterface = (*Server)(nil)

func NewRouter(database Pinger, imports *repository.ImportRepository, userID pgtype.UUID, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger, formatUUID(userID)))
	router.Use(recoverer(logger))
	return HandlerFromMux(&Server{database: database, imports: imports, userID: userID, logger: logger}, router)
}

func (s *Server) CreateImport(w http.ResponseWriter, r *http.Request) {
	var request CreateImportRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one URL")
		return
	}
	request.Url = strings.TrimSpace(request.Url)
	parsed, err := url.ParseRequestURI(request.Url)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || len(request.Url) > 4096 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_URL", "url must be an HTTP or HTTPS URL")
		return
	}
	status, err := s.imports.Create(r.Context(), s.userID, request.Url)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "create import", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusAccepted, importResponse(status))
}

func (s *Server) GetImport(w http.ResponseWriter, r *http.Request, importID string) {
	var parsedID pgtype.UUID
	if err := parsedID.Scan(importID); err != nil {
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
