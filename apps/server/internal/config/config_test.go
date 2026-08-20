package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/echonote?sslmode=disable")
	t.Setenv("APP_ENV", "test")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("WORKER_POLL_INTERVAL", "250ms")
	t.Setenv("WORKER_LEASE_TIMEOUT", "2m")
	t.Setenv("ECHONOTE_USER_ID", "fb48ddae-0ac8-4fb3-9e1a-f293ff938ed2")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerPort != 9090 || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected server config: %+v", cfg)
	}
	if cfg.WorkerPollInterval != 250*time.Millisecond || cfg.WorkerLeaseTimeout != 2*time.Minute {
		t.Fatalf("unexpected worker config: %+v", cfg)
	}
	if !cfg.UserID.Valid {
		t.Fatal("expected ECHONOTE_USER_ID to be parsed")
	}
}

func TestLoadRejectsInvalidUserID(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("ECHONOTE_USER_ID", "not-a-uuid")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid ECHONOTE_USER_ID to fail")
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("WORKER_POLL_INTERVAL", "later")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid WORKER_POLL_INTERVAL to fail")
	}
}
