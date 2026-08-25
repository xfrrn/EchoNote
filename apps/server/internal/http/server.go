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

	applogging "github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
)

type Pinger interface {
	Ping(context.Context) error
}

type Server struct {
	database Pinger
	imports  *repository.ImportRepository
	logger   *slog.Logger
}

var _ ServerInterface = (*Server)(nil)

func NewRouter(
	database Pinger,
	imports *repository.ImportRepository,
	users *repository.UserRepository,
	internalToken string,
	logger *slog.Logger,
) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(recoverer(logger))
	router.Use(identify(internalToken, users, logger))
	router.Use(requestLogger(logger))
	return HandlerFromMux(&Server{database: database, imports: imports, logger: logger}, router)
}

func (s *Server) GetLiveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, StatusResponse{Status: StatusResponseStatusOk})
}

func (s *Server) GetReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.database.Ping(ctx); err != nil {
		s.logger.WarnContext(r.Context(), "readiness check failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
		writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{Status: ReadinessResponseStatusUnavailable, Database: Down})
		return
	}
	writeJSON(w, http.StatusOK, ReadinessResponse{Status: ReadinessResponseStatusOk, Database: Up})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			requestID := middleware.GetReqID(r.Context())
			requestLog := logger.With("request_id", requestID, "user_id", formatUUID(requestUserID(r)))
			r = r.WithContext(applogging.WithLogger(r.Context(), requestLog))
			w.Header().Set("X-Request-ID", requestID)
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			requestLog.InfoContext(r.Context(), "http request",
				"method", r.Method, "path", r.URL.Path, "status", wrapped.Status(),
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
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func parseHTTPURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
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
	value := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func dbUUID(value [16]byte) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "http panic", "request_id", middleware.GetReqID(r.Context()),
						"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
					writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
