package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	issuerHeader  = "X-EchoNote-Auth-Issuer"
	subjectHeader = "X-EchoNote-Auth-Subject"
	emailHeader   = "X-EchoNote-Auth-Email"
)

type userContextKey struct{}

func identify(internalToken string, users *repository.UserRepository, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if len(provided) != len(internalToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(internalToken)) != 1 {
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "internal authentication failed")
				return
			}
			issuer, subject, email := strings.TrimSpace(r.Header.Get(issuerHeader)), strings.TrimSpace(r.Header.Get(subjectHeader)), strings.TrimSpace(r.Header.Get(emailHeader))
			if issuer == "" || subject == "" || len(issuer) > 2048 || len(subject) > 512 || len(email) > 320 {
				writeAPIError(w, http.StatusUnauthorized, "INVALID_IDENTITY", "authenticated identity is missing or invalid")
				return
			}
			userID, err := users.Resolve(r.Context(), issuer, subject, email)
			if errors.Is(err, pgx.ErrNoRows) {
				writeAPIError(w, http.StatusForbidden, "USER_DISABLED", "user access is disabled")
				return
			}
			if err != nil {
				logger.ErrorContext(r.Context(), "resolve authenticated user", "error", err)
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, userID)))
		})
	}
}

func requestUserID(r *http.Request) pgtype.UUID {
	userID, _ := r.Context().Value(userContextKey{}).(pgtype.UUID)
	return userID
}
