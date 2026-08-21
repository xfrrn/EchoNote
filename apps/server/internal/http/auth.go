package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	authn "github.com/Actify/echonote/apps/server/internal/auth"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AuthConfig struct {
	PublicOrigin      string
	SessionTTL        time.Duration
	PasswordCost      int
	SecureCookies     bool
	DevelopmentUserID pgtype.UUID
}

type authContextKey struct{}

func (config AuthConfig) dummyPasswordHash() (string, error) {
	return authn.HashPassword("invalid login password", config.PasswordCost)
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "username and password are required")
		return
	}
	if s.auth == nil {
		s.logger.ErrorContext(r.Context(), "login unavailable", "request_id", middleware.GetReqID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	_, normalized, normalizeErr := authn.NormalizeUsername(request.Username)
	user, err := s.auth.UserForLogin(r.Context(), normalized)
	if normalizeErr != nil || errors.Is(err, pgx.ErrNoRows) {
		authn.VerifyPassword(s.dummyPasswordHash, request.Password)
		writeAPIError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "login lookup", "request_id", middleware.GetReqID(r.Context()), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if user.PasswordHash == nil || !authn.VerifyPassword(*user.PasswordHash, request.Password) {
		writeAPIError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
		return
	}

	token, tokenHash, err := authn.NewSessionToken()
	if err != nil {
		s.logger.ErrorContext(r.Context(), "generate session", "request_id", middleware.GetReqID(r.Context()), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	expiresAt, err := s.auth.CreateSession(r.Context(), user.ID, tokenHash, s.authConfig.SessionTTL)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "create session", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(user.ID), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	setSessionCookie(w, token, expiresAt, s.authConfig.SecureCookies)
	response := LoginResponse{User: UserResponse{Id: formatUUID(user.ID), Username: *user.Username}, ExpiresAt: expiresAt}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(authn.SessionCookieName)
	if err == nil {
		tokenHash, hashErr := authn.HashSessionToken(cookie.Value)
		if hashErr == nil {
			if err := s.auth.RevokeSession(r.Context(), tokenHash); err != nil {
				s.logger.ErrorContext(r.Context(), "revoke session", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r)), "error", err)
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}
		}
	}
	clearSessionCookie(w, s.authConfig.SecureCookies)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := requestUser(r)
	writeJSON(w, http.StatusOK, LoginResponse{
		User:      UserResponse{Id: formatUUID(user.ID), Username: user.Username},
		ExpiresAt: user.ExpiresAt,
	})
}

func authenticate(authRepository *repository.AuthRepository, developmentUserID pgtype.UUID, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/api/v1/auth/login" {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(authn.SessionCookieName)
			if errors.Is(err, http.ErrNoCookie) && developmentUserID.Valid {
				user := repository.SessionUser{ID: developmentUserID, Username: "development", ExpiresAt: time.Now().Add(24 * time.Hour)}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, user)))
				return
			}
			if err != nil || authRepository == nil {
				writeAPIError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
				return
			}
			tokenHash, err := authn.HashSessionToken(cookie.Value)
			if err != nil {
				writeAPIError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
				return
			}
			user, err := authRepository.AuthenticateSession(r.Context(), tokenHash)
			if errors.Is(err, pgx.ErrNoRows) {
				writeAPIError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
				return
			}
			if err != nil {
				logger.ErrorContext(r.Context(), "authenticate session", "request_id", middleware.GetReqID(r.Context()), "error", err)
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, user)))
		})
	}
}

func sameOrigin(publicOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicOrigin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.Header.Get("Origin") != publicOrigin {
				writeAPIError(w, http.StatusForbidden, "ORIGIN_FORBIDDEN", "request origin is not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func apiNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) >= len("/api/") && r.URL.Path[:len("/api/")] == "/api/" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func requestUser(r *http.Request) repository.SessionUser {
	user, _ := r.Context().Value(authContextKey{}).(repository.SessionUser)
	return user
}

func requestUserID(r *http.Request) pgtype.UUID {
	return requestUser(r).ID
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: authn.SessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(time.Until(expiresAt).Seconds()), Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: authn.SessionCookieName, Path: "/", Expires: time.Unix(1, 0), MaxAge: -1,
		Secure: secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}
