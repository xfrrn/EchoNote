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
	t.Setenv("ASR_STANDARD_MODEL", "")
	t.Setenv("ASR_QUALITY_MODEL", "")
	t.Setenv("SESSION_TTL", "24h")
	t.Setenv("PASSWORD_BCRYPT_COST", "11")
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
	if cfg.ASRStandardModel != "paraformer-v2" || cfg.ASRQualityModel != "fun-asr" {
		t.Fatalf("unexpected ASR models: standard=%q quality=%q", cfg.ASRStandardModel, cfg.ASRQualityModel)
	}
	if cfg.SessionTTL != 24*time.Hour || cfg.PasswordBcryptCost != 11 {
		t.Fatalf("unexpected auth config: %+v", cfg)
	}
	if cfg.EmbeddingEndpoint != "https://dashscope.aliyuncs.com" {
		t.Fatalf("unexpected embedding endpoint: %q", cfg.EmbeddingEndpoint)
	}
	if cfg.LLMEndpoint != "https://dashscope.aliyuncs.com/compatible-mode/v1" || cfg.LLMModel != "qwen-plus" {
		t.Fatalf("unexpected LLM defaults: endpoint=%q model=%q", cfg.LLMEndpoint, cfg.LLMModel)
	}
	if !cfg.DevelopmentUserID.Valid {
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

func TestLoadRejectsInvalidASRModels(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("ASR_STANDARD_MODEL", "fun-asr")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid standard ASR model to fail")
	}
	t.Setenv("ASR_STANDARD_MODEL", "paraformer-v2")
	t.Setenv("ASR_QUALITY_MODEL", "paraformer-v2")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid quality ASR model to fail")
	}
}

func TestLoadRejectsInvalidUserID(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("ECHONOTE_USER_ID", "not-a-uuid")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid ECHONOTE_USER_ID to fail")
	}
}

func TestLoadAllowsSessionAuthInDevelopment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote")
	t.Setenv("APP_ENV", "development")
	t.Setenv("ECHONOTE_USER_ID", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DevelopmentUserID.Valid {
		t.Fatal("expected development identity fallback to be opt-in")
	}
}

func TestLoadRejectsProductionDevelopmentIdentity(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote?sslmode=verify-full")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://notes.example.com")
	t.Setenv("ECHONOTE_USER_ID", "fb48ddae-0ac8-4fb3-9e1a-f293ff938ed2")
	if _, err := Load(); err == nil {
		t.Fatal("expected production ECHONOTE_USER_ID to fail")
	}
}

func TestLoadRequiresSecureProductionOrigin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/echonote?sslmode=verify-full")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("ECHONOTE_USER_ID", "")
	t.Setenv("PUBLIC_ORIGIN", "http://notes.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure staging PUBLIC_ORIGIN to fail")
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

func TestProductionServiceValidation(t *testing.T) {
	setSecureEnvironment(t)
	t.Setenv("EMBEDDING_PROVIDER", "aliyun")
	t.Setenv("EMBEDDING_API_KEY", "embedding-test-key")
	t.Setenv("LLM_PROVIDER", "aliyun")
	t.Setenv("LLM_API_KEY", "llm-test-key")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err != nil {
		t.Fatalf("API config: %v", err)
	}
	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected worker config without ASR and storage secrets to fail")
	}

	t.Setenv("ASR_PROVIDER", "aliyun")
	t.Setenv("ASR_API_KEY", "asr-test-key")
	t.Setenv("STORAGE_PROVIDER", "aliyun_oss")
	t.Setenv("STORAGE_REGION", "cn-beijing")
	t.Setenv("STORAGE_BUCKET", "echonote-production")
	t.Setenv("STORAGE_ACCESS_KEY", "storage-test-id")
	t.Setenv("STORAGE_SECRET_KEY", "storage-test-secret")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateWorker(); err != nil {
		t.Fatalf("worker config: %v", err)
	}
	if cfg.ListenAddress() != "127.0.0.1:8080" || cfg.WorkerTempMaxAge != 24*time.Hour {
		t.Fatalf("unexpected production defaults: address=%q temp_age=%s", cfg.ListenAddress(), cfg.WorkerTempMaxAge)
	}
}

func TestLoadRejectsUnsafeProductionDatabase(t *testing.T) {
	cases := map[string]string{
		"placeholder": "postgres://echonote_api:CHANGE_ME@db.example.com/echonote?sslmode=verify-full&pool_max_conns=10",
		"superuser":   "postgres://postgres@db.example.com/echonote?sslmode=verify-full&pool_max_conns=10",
		"weak TLS":    "postgres://echonote_api@db.example.com/echonote?sslmode=require&pool_max_conns=10",
		"no budget":   "postgres://echonote_api@db.example.com/echonote?sslmode=verify-full&pool_max_conns=20",
		"wrong DB":    "postgres://echonote_api@db.example.com/autoup?sslmode=verify-full&pool_max_conns=10",
	}
	for name, databaseURL := range cases {
		t.Run(name, func(t *testing.T) {
			setSecureEnvironment(t)
			t.Setenv("DATABASE_URL", databaseURL)
			if _, err := Load(); err == nil {
				t.Fatal("expected unsafe production DATABASE_URL to fail")
			}
		})
	}
}

func TestProductionValidationRejectsPlaceholderProviderKey(t *testing.T) {
	setSecureEnvironment(t)
	t.Setenv("EMBEDDING_PROVIDER", "aliyun")
	t.Setenv("EMBEDDING_API_KEY", "CHANGE_ME")
	t.Setenv("LLM_PROVIDER", "aliyun")
	t.Setenv("LLM_API_KEY", "llm-test-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAPI(); err == nil {
		t.Fatal("expected placeholder Provider key to fail")
	}
}

func setSecureEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ASR_PROVIDER", "ASR_API_KEY", "ASR_STANDARD_MODEL", "ASR_QUALITY_MODEL", "STORAGE_PROVIDER", "STORAGE_REGION", "STORAGE_BUCKET", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY", "EMBEDDING_PROVIDER", "EMBEDDING_API_KEY", "LLM_PROVIDER", "LLM_API_KEY"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://notes.example.com")
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("DATABASE_URL", "postgres://echonote_api@db.example.com/echonote?sslmode=verify-full&pool_max_conns=10")
	t.Setenv("EXPECTED_DATABASE_NAME", "echonote")
	t.Setenv("DATABASE_CONNECTION_BUDGET", "20")
	t.Setenv("TRANSCRIPTION_ENABLED", "true")
	t.Setenv("ECHONOTE_USER_ID", "")
}
