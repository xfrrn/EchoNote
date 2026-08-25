package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Config struct {
	Environment        string
	ServerHost         string
	ServerPort         int
	DatabaseURL        string
	ExpectedDatabase   string
	DatabaseBudget     int
	LogLevel           slog.Level
	WorkerPollInterval time.Duration
	WorkerLeaseTimeout time.Duration
	WorkerTempMaxAge   time.Duration
	ASRPollInterval    time.Duration
	ASRProvider        string
	ASRAPIKey          string
	ASREndpoint        string
	ASRStandardModel   string
	ASRQualityModel    string
	EmbeddingProvider  string
	EmbeddingAPIKey    string
	EmbeddingEndpoint  string
	LLMProvider        string
	LLMAPIKey          string
	LLMEndpoint        string
	LLMModel           string
	StorageProvider    string
	StorageRegion      string
	StorageEndpoint    string
	StorageBucket      string
	StorageAccessKey   string
	StorageSecretKey   string
	FFmpegPath         string
	FFprobePath        string
	TranscriptionAPI   bool
	OwnerID            pgtype.UUID
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        envOrDefault("APP_ENV", "development"),
		ServerHost:         envOrDefault("SERVER_HOST", "127.0.0.1"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		ExpectedDatabase:   strings.TrimSpace(os.Getenv("EXPECTED_DATABASE_NAME")),
		WorkerPollInterval: time.Second,
		WorkerLeaseTimeout: 5 * time.Minute,
		WorkerTempMaxAge:   24 * time.Hour,
		ASRPollInterval:    5 * time.Second,
		ASRProvider:        strings.ToLower(strings.TrimSpace(os.Getenv("ASR_PROVIDER"))),
		ASRAPIKey:          strings.TrimSpace(os.Getenv("ASR_API_KEY")),
		ASREndpoint:        envOrDefault("ASR_ENDPOINT", "https://dashscope.aliyuncs.com"),
		ASRStandardModel:   strings.ToLower(envOrDefault("ASR_STANDARD_MODEL", "paraformer-v2")),
		ASRQualityModel:    strings.ToLower(envOrDefault("ASR_QUALITY_MODEL", "fun-asr")),
		EmbeddingProvider:  strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER"))),
		EmbeddingAPIKey:    strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		EmbeddingEndpoint:  envOrDefault("EMBEDDING_ENDPOINT", "https://dashscope.aliyuncs.com"),
		LLMProvider:        strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER"))),
		LLMAPIKey:          strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMEndpoint:        envOrDefault("LLM_ENDPOINT", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		LLMModel:           envOrDefault("LLM_MODEL", "qwen-plus"),
		StorageProvider:    strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))),
		StorageRegion:      strings.TrimSpace(os.Getenv("STORAGE_REGION")),
		StorageEndpoint:    strings.TrimSpace(os.Getenv("STORAGE_ENDPOINT")),
		StorageBucket:      strings.TrimSpace(os.Getenv("STORAGE_BUCKET")),
		StorageAccessKey:   strings.TrimSpace(os.Getenv("STORAGE_ACCESS_KEY")),
		StorageSecretKey:   strings.TrimSpace(os.Getenv("STORAGE_SECRET_KEY")),
		FFmpegPath:         envOrDefault("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:        envOrDefault("FFPROBE_PATH", "ffprobe"),
	}

	if cfg.Environment != "development" && cfg.Environment != "test" && cfg.Environment != "staging" && cfg.Environment != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, staging, or production")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	ownerID := strings.TrimSpace(os.Getenv("ECHONOTE_OWNER_ID"))
	if ownerID == "" {
		return Config{}, fmt.Errorf("ECHONOTE_OWNER_ID is required")
	}
	if err := cfg.OwnerID.Scan(ownerID); err != nil {
		return Config{}, fmt.Errorf("ECHONOTE_OWNER_ID must be a UUID")
	}
	if cfg.SecureEnvironment() {
		budget, err := strconv.Atoi(strings.TrimSpace(os.Getenv("DATABASE_CONNECTION_BUDGET")))
		if err != nil || budget < 2 {
			return Config{}, fmt.Errorf("DATABASE_CONNECTION_BUDGET must be an integer greater than 1")
		}
		cfg.DatabaseBudget = budget
		if err := validateSecureDatabaseURL(cfg.DatabaseURL, cfg.ExpectedDatabase, budget); err != nil {
			return Config{}, err
		}
	}

	port, err := strconv.Atoi(envOrDefault("SERVER_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("SERVER_PORT must be an integer between 1 and 65535")
	}
	cfg.ServerPort = port
	if err := cfg.LogLevel.UnmarshalText([]byte(envOrDefault("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}

	if cfg.WorkerPollInterval, err = positiveDuration("WORKER_POLL_INTERVAL", cfg.WorkerPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.WorkerLeaseTimeout, err = positiveDuration("WORKER_LEASE_TIMEOUT", cfg.WorkerLeaseTimeout); err != nil {
		return Config{}, err
	}
	if cfg.WorkerTempMaxAge, err = positiveDuration("WORKER_TEMP_MAX_AGE", cfg.WorkerTempMaxAge); err != nil {
		return Config{}, err
	}
	if cfg.ASRPollInterval, err = positiveDuration("ASR_POLL_INTERVAL", cfg.ASRPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.ASRProvider != "" && cfg.ASRProvider != "aliyun" {
		return Config{}, fmt.Errorf("ASR_PROVIDER must be aliyun")
	}
	if cfg.ASRStandardModel != "paraformer-v2" {
		return Config{}, fmt.Errorf("ASR_STANDARD_MODEL must be paraformer-v2")
	}
	if cfg.ASRQualityModel != "fun-asr" {
		return Config{}, fmt.Errorf("ASR_QUALITY_MODEL must be fun-asr")
	}
	if cfg.StorageProvider != "" && cfg.StorageProvider != "aliyun_oss" {
		return Config{}, fmt.Errorf("STORAGE_PROVIDER must be aliyun_oss")
	}
	if cfg.EmbeddingProvider != "" && cfg.EmbeddingProvider != "aliyun" {
		return Config{}, fmt.Errorf("EMBEDDING_PROVIDER must be aliyun")
	}
	if cfg.LLMProvider != "" && cfg.LLMProvider != "aliyun" {
		return Config{}, fmt.Errorf("LLM_PROVIDER must be aliyun")
	}
	if cfg.StorageEndpoint != "" {
		endpoint, parseErr := url.Parse(cfg.StorageEndpoint)
		if parseErr != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return Config{}, fmt.Errorf("STORAGE_ENDPOINT must be an HTTPS URL without credentials")
		}
	}
	if cfg.ASRProvider != "" {
		endpoint, parseErr := url.Parse(cfg.ASREndpoint)
		if parseErr != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return Config{}, fmt.Errorf("ASR_ENDPOINT must be an HTTPS URL without credentials")
		}
	}
	if cfg.EmbeddingProvider != "" {
		endpoint, parseErr := url.Parse(cfg.EmbeddingEndpoint)
		if parseErr != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return Config{}, fmt.Errorf("EMBEDDING_ENDPOINT must be an HTTPS URL without credentials")
		}
	}
	if cfg.LLMProvider != "" {
		endpoint, parseErr := url.Parse(cfg.LLMEndpoint)
		if parseErr != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return Config{}, fmt.Errorf("LLM_ENDPOINT must be an HTTPS URL without credentials")
		}
	}
	rawTranscriptionAPI := strings.TrimSpace(os.Getenv("TRANSCRIPTION_ENABLED"))
	if rawTranscriptionAPI == "" {
		if cfg.SecureEnvironment() {
			return Config{}, fmt.Errorf("TRANSCRIPTION_ENABLED is required in staging and production")
		}
		cfg.TranscriptionAPI = cfg.TranscriptionEnabled()
	} else {
		cfg.TranscriptionAPI, err = strconv.ParseBool(rawTranscriptionAPI)
		if err != nil {
			return Config{}, fmt.Errorf("TRANSCRIPTION_ENABLED must be true or false")
		}
	}

	return cfg, nil
}

func (cfg Config) SecureEnvironment() bool {
	return cfg.Environment == "staging" || cfg.Environment == "production"
}

func (cfg Config) ListenAddress() string {
	if cfg.ServerHost == "" {
		return ":" + strconv.Itoa(cfg.ServerPort)
	}
	return net.JoinHostPort(cfg.ServerHost, strconv.Itoa(cfg.ServerPort))
}

func (cfg Config) ValidateAPI() error {
	host := net.ParseIP(cfg.ServerHost)
	if host == nil || !host.IsLoopback() {
		return fmt.Errorf("SERVER_HOST must be a loopback IP")
	}
	if !cfg.SecureEnvironment() {
		return nil
	}
	if !cfg.TranscriptionAPI {
		return fmt.Errorf("TRANSCRIPTION_ENABLED must be true in staging and production")
	}
	if err := cfg.ValidateEmbedding(); err != nil {
		return err
	}
	return cfg.ValidateLLM()
}

func (cfg Config) ValidateWorker() error {
	if !cfg.SecureEnvironment() {
		return nil
	}
	if err := cfg.ValidateTranscription(); err != nil {
		return err
	}
	if err := cfg.ValidateEmbedding(); err != nil {
		return err
	}
	return cfg.ValidateLLM()
}

func (cfg Config) ValidateTranscription() error {
	missing := make([]string, 0, 7)
	for key, value := range map[string]string{
		"ASR_PROVIDER": cfg.ASRProvider, "ASR_API_KEY": cfg.ASRAPIKey,
		"STORAGE_PROVIDER": cfg.StorageProvider, "STORAGE_REGION": cfg.StorageRegion,
		"STORAGE_BUCKET": cfg.StorageBucket, "STORAGE_ACCESS_KEY": cfg.StorageAccessKey,
		"STORAGE_SECRET_KEY": cfg.StorageSecretKey,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if cfg.SecureEnvironment() {
		for key, value := range map[string]string{
			"ASR_API_KEY": cfg.ASRAPIKey, "STORAGE_ACCESS_KEY": cfg.StorageAccessKey, "STORAGE_SECRET_KEY": cfg.StorageSecretKey,
		} {
			if placeholder(value) {
				missing = append(missing, key)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("transcription requires %s", strings.Join(missing, ", "))
	}
	return nil
}

func (cfg Config) TranscriptionEnabled() bool {
	return cfg.ValidateTranscription() == nil
}

func (cfg Config) ValidateEmbedding() error {
	missing := make([]string, 0, 2)
	if cfg.EmbeddingProvider == "" {
		missing = append(missing, "EMBEDDING_PROVIDER")
	}
	if cfg.EmbeddingAPIKey == "" {
		missing = append(missing, "EMBEDDING_API_KEY")
	}
	if cfg.SecureEnvironment() && placeholder(cfg.EmbeddingAPIKey) {
		missing = append(missing, "EMBEDDING_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("semantic search requires %s", strings.Join(missing, ", "))
	}
	return nil
}

func (cfg Config) EmbeddingEnabled() bool {
	return cfg.ValidateEmbedding() == nil
}

func (cfg Config) ValidateLLM() error {
	missing := make([]string, 0, 2)
	if cfg.LLMProvider == "" {
		missing = append(missing, "LLM_PROVIDER")
	}
	if cfg.LLMAPIKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	if cfg.SecureEnvironment() && placeholder(cfg.LLMAPIKey) {
		missing = append(missing, "LLM_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("AI requires %s", strings.Join(missing, ", "))
	}
	return nil
}

func (cfg Config) LLMEnabled() bool {
	return cfg.ValidateLLM() == nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func positiveDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := envOrDefault(key, fallback.String())
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return duration, nil
}

func validateSecureDatabaseURL(raw, expectedDatabase string, budget int) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return fmt.Errorf("DATABASE_URL must be a PostgreSQL URL in staging and production")
	}
	if parsed.User == nil || parsed.User.Username() == "" || strings.EqualFold(parsed.User.Username(), "postgres") {
		return fmt.Errorf("DATABASE_URL must use a named non-postgres role in staging and production")
	}
	if password, ok := parsed.User.Password(); ok && placeholder(password) {
		return fmt.Errorf("DATABASE_URL must not contain a placeholder password")
	}
	if expectedDatabase != "echonote" && !strings.HasPrefix(expectedDatabase, "echonote_") {
		return fmt.Errorf("EXPECTED_DATABASE_NAME must be echonote or start with echonote_")
	}
	if strings.TrimPrefix(parsed.Path, "/") != expectedDatabase {
		return fmt.Errorf("DATABASE_URL database must match EXPECTED_DATABASE_NAME")
	}
	query := parsed.Query()
	if query.Get("sslmode") != "verify-full" {
		return fmt.Errorf("DATABASE_URL must set sslmode=verify-full in staging and production")
	}
	poolMax, err := strconv.Atoi(query.Get("pool_max_conns"))
	if err != nil || poolMax < 1 || poolMax >= budget {
		return fmt.Errorf("DATABASE_URL pool_max_conns must be positive and below DATABASE_CONNECTION_BUDGET")
	}
	return nil
}

func placeholder(value string) bool {
	value = strings.TrimSpace(value)
	return strings.EqualFold(value, "CHANGE_ME") || strings.EqualFold(value, "CHANGEME")
}
