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
)

type pingerFunc func(context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestHealthEndpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		path       string
		pingError  error
		wantStatus int
		wantBody   string
	}{
		{path: "/healthz", wantStatus: http.StatusOK, wantBody: `"status":"ok"`},
		{path: "/readyz", wantStatus: http.StatusOK, wantBody: `"database":"up"`},
		{path: "/readyz", pingError: errors.New("offline"), wantStatus: http.StatusServiceUnavailable, wantBody: `"database":"down"`},
	}
	for _, test := range tests {
		router := NewRouter(pingerFunc(func(context.Context) error { return test.pingError }), nil, nil, "secret", logger)
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantBody) {
			t.Fatalf("%s: status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Request-ID") == "" {
			t.Fatal("X-Request-ID is missing")
		}
	}
}

func TestAPIRequiresInternalAuthentication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pingerFunc(func(context.Context) error { return nil }), nil, nil, "0123456789abcdef0123456789abcdef", logger)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/transcriptions", strings.NewReader(`{"url":"https://example.com/audio.mp3"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestParseHTTPURL(t *testing.T) {
	if _, err := parseHTTPURL("https://example.com/audio.mp3"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"file:///tmp/audio.mp3", "https://user:pass@example.com/audio.mp3", "not a url"} {
		if _, err := parseHTTPURL(raw); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}
