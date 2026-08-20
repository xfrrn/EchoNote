package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Pinger interface {
	Ping(context.Context) error
}

type Server struct {
	database Pinger
	logger   *slog.Logger
}

var _ ServerInterface = (*Server)(nil)

func NewRouter(database Pinger, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger))
	router.Use(recoverer(logger))
	return HandlerFromMux(&Server{database: database, logger: logger}, router)
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

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			requestID := middleware.GetReqID(r.Context())
			w.Header().Set("X-Request-ID", requestID)
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			logger.InfoContext(r.Context(), "http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.Status(),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		})
	}
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
