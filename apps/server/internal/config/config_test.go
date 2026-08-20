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
	t.Setenv("ASR_POLL_INTERVAL", "3s")
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
	if cfg.ASRPollInterval != 3*time.Second || cfg.FFmpegPath != "ffmpeg" || cfg.FFprobePath != "ffprobe" {
		t.Fatalf("unexpected transcription defaults: %+v", cfg)
	}
	if cfg.EmbeddingEndpoint != "https://dashscope.aliyuncs.com" {
		t.Fatalf("unexpected embedding endpoint: %q", cfg.EmbeddingEndpoint)
	}
	if cfg.LLMEndpoint != "https://dashscope.aliyuncs.com/compatible-mode/v1" || cfg.LLMModel != "qwen-plus" {
		t.Fatalf("unexpected LLM defaults: endpoint=%q model=%q", cfg.LLMEndpoint, cfg.LLMModel)
	}
	if !cfg.UserID.Valid {
		t.Fatal("expected ECHONOTE_USER_ID to be parsed")
	}
}

func TestValidateEmbedding(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("EMBEDDING_PROVIDER", "aliyun")
	t.Setenv("EMBEDDING_API_KEY", "test-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateEmbedding(); err != nil || !cfg.EmbeddingEnabled() {
		t.Fatalf("enabled=%v err=%v", cfg.EmbeddingEnabled(), err)
	}
}

func TestValidateLLM(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("LLM_PROVIDER", "aliyun")
	t.Setenv("LLM_API_KEY", "test-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateLLM(); err != nil || !cfg.LLMEnabled() {
		t.Fatalf("enabled=%v err=%v", cfg.LLMEnabled(), err)
	}
}

func TestValidateTranscription(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
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
	if err := cfg.ValidateTranscription(); err != nil || !cfg.TranscriptionEnabled() {
		t.Fatalf("enabled=%v err=%v", cfg.TranscriptionEnabled(), err)
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

func TestLoadRejectsInsecureStorageEndpoint(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("STORAGE_ENDPOINT", "http://access:secret@oss.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure STORAGE_ENDPOINT to fail")
	}
}

func TestLoadRejectsInsecureEmbeddingEndpoint(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("EMBEDDING_PROVIDER", "aliyun")
	t.Setenv("EMBEDDING_ENDPOINT", "http://key@example.com/api/v1")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure EMBEDDING_ENDPOINT to fail")
	}
}

func TestLoadRejectsInsecureLLMEndpoint(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("LLM_PROVIDER", "aliyun")
	t.Setenv("LLM_ENDPOINT", "http://key@example.com/api/v1")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure LLM_ENDPOINT to fail")
	}
}
