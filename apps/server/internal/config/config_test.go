package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadAndValidateAPI(t *testing.T) {
	baseEnvironment(t)
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("WORKER_POLL_INTERVAL", "250ms")
	t.Setenv("ECHONOTE_INTERNAL_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("SNAPANY_API_KEY", "test-snapany-key")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress() != "127.0.0.1:9090" || cfg.LogLevel != slog.LevelDebug || cfg.WorkerPollInterval != 250*time.Millisecond {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.ASRQualityModel != "fun-asr" || cfg.SnapAnyAPIKey != "test-snapany-key" {
		t.Fatalf("unexpected provider config: %+v", cfg)
	}
}

func TestValidateAPIRejectsPublicHostAndWeakToken(t *testing.T) {
	baseEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err == nil {
		t.Fatal("expected missing internal token to fail")
	}
	t.Setenv("ECHONOTE_INTERNAL_TOKEN", "CHANGE_ME_0123456789abcdef0123456789")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err == nil {
		t.Fatal("expected placeholder internal token to fail")
	}
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("ECHONOTE_INTERNAL_TOKEN", "0123456789abcdef0123456789abcdef")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err == nil {
		t.Fatal("expected public internal API host to fail")
	}
}

func TestValidateTranscription(t *testing.T) {
	baseEnvironment(t)
	t.Setenv("ASR_PROVIDER", "aliyun")
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("STORAGE_PROVIDER", "aliyun_oss")
	t.Setenv("STORAGE_REGION", "cn-beijing")
	t.Setenv("STORAGE_BUCKET", "echonote-test")
	t.Setenv("STORAGE_ACCESS_KEY", "test-id")
	t.Setenv("STORAGE_SECRET_KEY", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateWorker(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]func(*testing.T){
		"missing database": func(t *testing.T) { t.Setenv("DATABASE_URL", "") },
		"invalid duration": func(t *testing.T) { t.Setenv("WORKER_POLL_INTERVAL", "later") },
		"invalid model":    func(t *testing.T) { t.Setenv("ASR_QUALITY_MODEL", "paraformer-v2") },
		"insecure storage": func(t *testing.T) { t.Setenv("STORAGE_ENDPOINT", "http://oss.example.com") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			baseEnvironment(t)
			mutate(t)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
}

func TestProductionDatabaseValidation(t *testing.T) {
	baseEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://echonote_api@db.example.com/echonote?sslmode=verify-full&pool_max_conns=10")
	t.Setenv("EXPECTED_DATABASE_NAME", "echonote")
	t.Setenv("DATABASE_CONNECTION_BUDGET", "20")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://postgres@db.example.com/echonote?sslmode=verify-full&pool_max_conns=10")
	if _, err := Load(); err == nil {
		t.Fatal("expected postgres superuser to fail")
	}
}

func baseEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_ENV", "SERVER_HOST", "SERVER_PORT", "LOG_LEVEL", "DATABASE_URL", "EXPECTED_DATABASE_NAME", "DATABASE_CONNECTION_BUDGET",
		"ECHONOTE_INTERNAL_TOKEN", "WORKER_POLL_INTERVAL", "WORKER_LEASE_TIMEOUT", "WORKER_TEMP_MAX_AGE", "ASR_POLL_INTERVAL",
		"ASR_PROVIDER", "ASR_API_KEY", "ASR_ENDPOINT", "ASR_QUALITY_MODEL",
		"SNAPANY_API_KEY",
		"STORAGE_PROVIDER", "STORAGE_REGION", "STORAGE_ENDPOINT", "STORAGE_BUCKET", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
}
