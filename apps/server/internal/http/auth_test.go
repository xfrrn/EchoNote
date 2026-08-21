package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBusinessRoutesRequireSessionAndSameOrigin(t *testing.T) {
	router := NewRouter(
		pingerFunc(func(context.Context) error { return nil }),
		nil, nil, nil, nil, nil, nil, nil, nil, false,
		AuthConfig{PublicOrigin: "https://notes.example.test", PasswordCost: 4},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/episodes", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}

	for _, path := range []string{"/api/v1/auth/login", "/api/v1/captures"} {
		request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Origin", "https://evil.example.test")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "ORIGIN_FORBIDDEN") {
			t.Fatalf("cross-origin path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	response := httptest.NewRecorder()
	setSessionCookie(response, "secret", expiresAt, true)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.Expires.IsZero() || cookie.MaxAge <= 0 {
		t.Fatalf("cookie=%+v", cookie)
	}
}
