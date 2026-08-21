package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	authn "github.com/Actify/echonote/apps/server/internal/auth"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/repository"
)

func TestSessionHTTPFlow(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-auth-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	authRepository := repository.NewAuthRepository(pool)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "correct horse battery staple"
	passwordHash, err := authn.HashPassword(password, 4)
	if err != nil {
		t.Fatal(err)
	}
	username := "auth_" + suffix
	user, err := authRepository.CreateUser(ctx, username, username, passwordHash)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID) }()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	config := AuthConfig{
		PublicOrigin: "https://notes.example.test", SessionTTL: time.Hour,
		PasswordCost: 4, SecureCookies: true,
	}
	newRouter := func() http.Handler {
		return NewRouter(pool, nil, nil, nil, nil, nil, nil, nil, authRepository, false, config, logger)
	}
	router := newRouter()

	wrongUser := serveLogin(router, config.PublicOrigin, "missing_"+suffix, password)
	wrongPassword := serveLogin(router, config.PublicOrigin, username, "incorrect password value")
	if wrongUser.Code != http.StatusUnauthorized || wrongPassword.Code != http.StatusUnauthorized || wrongUser.Body.String() != wrongPassword.Body.String() {
		t.Fatalf("credential errors differ: user=%d %q password=%d %q", wrongUser.Code, wrongUser.Body.String(), wrongPassword.Code, wrongPassword.Body.String())
	}

	login := serveLogin(router, config.PublicOrigin, username, password)
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" || cookies[0].Domain != "" {
		t.Fatalf("login cookies=%+v", cookies)
	}
	sessionCookie := cookies[0]

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	newRouter().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), username) {
		t.Fatalf("restarted router me status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.Header.Set("Origin", config.PublicOrigin)
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout status=%d cookies=%+v body=%s", response.Code, response.Result().Cookies(), response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d body=%s", response.Code, response.Body.String())
	}

	secondLogin := serveLogin(router, config.PublicOrigin, username, password)
	if secondLogin.Code != http.StatusOK {
		t.Fatalf("second login status=%d body=%s", secondLogin.Code, secondLogin.Body.String())
	}
	resetPassword := "new correct horse battery staple"
	resetHash, err := authn.HashPassword(resetPassword, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authRepository.ResetPassword(ctx, username, resetHash); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(secondLogin.Result().Cookies()[0])
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || serveLogin(router, config.PublicOrigin, username, password).Code != http.StatusUnauthorized {
		t.Fatal("password reset did not revoke sessions and old credentials")
	}
	if serveLogin(router, config.PublicOrigin, username, resetPassword).Code != http.StatusOK {
		t.Fatal("reset password cannot create a new session")
	}

	expiredToken, expiredHash, err := authn.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, created_at, last_seen_at, expires_at)
		VALUES ($1, $2, now() - interval '2 hours', now() - interval '2 hours', now() - interval '1 hour')
	`, user.ID, expiredHash[:]); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(&http.Cookie{Name: authn.SessionCookieName, Value: expiredToken})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status=%d body=%s", response.Code, response.Body.String())
	}

	logOutput := logs.String()
	if strings.Contains(logOutput, password) || strings.Contains(logOutput, resetPassword) || strings.Contains(logOutput, sessionCookie.Value) || strings.Contains(logOutput, "Cookie") {
		t.Fatalf("sensitive authentication material appeared in logs: %s", logOutput)
	}
}

func serveLogin(router http.Handler, origin, username, password string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
