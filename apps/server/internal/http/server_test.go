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

func (f pingerFunc) Ping(ctx context.Context) error {
	return f(ctx)
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
			router := NewRouter(pingerFunc(func(context.Context) error { return test.pingError }), logger)
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
