package config

import (
	"fmt"
	"log/slog"
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
	ServerPort         int
	DatabaseURL        string
	PublicOrigin       string
	LogLevel           slog.Level
	SessionTTL         time.Duration
	PasswordBcryptCost int
	WorkerPollInterval time.Duration
	WorkerLeaseTimeout time.Duration
	ASRPollInterval    time.Duration
	ASRProvider        string
	ASRAPIKey          string
	ASREndpoint        string
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
	DevelopmentUserID  pgtype.UUID
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        envOrDefault("APP_ENV", "development"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PublicOrigin:       strings.TrimSuffix(strings.TrimSpace(os.Getenv("PUBLIC_ORIGIN")), "/"),
		SessionTTL:         30 * 24 * time.Hour,
		WorkerPollInterval: time.Second,
		WorkerLeaseTimeout: 5 * time.Minute,
		ASRPollInterval:    5 * time.Second,
		ASRProvider:        strings.ToLower(strings.TrimSpace(os.Getenv("ASR_PROVIDER"))),
		ASRAPIKey:          strings.TrimSpace(os.Getenv("ASR_API_KEY")),
		ASREndpoint:        envOrDefault("ASR_ENDPOINT", "https://dashscope.aliyuncs.com"),
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
	developmentUserID := strings.TrimSpace(os.Getenv("ECHONOTE_USER_ID"))
	if cfg.Environment == "staging" || cfg.Environment == "production" {
		if developmentUserID != "" {
			return Config{}, fmt.Errorf("ECHONOTE_USER_ID is forbidden in staging and production")
		}
		if cfg.PublicOrigin == "" {
			return Config{}, fmt.Errorf("PUBLIC_ORIGIN is required in staging and production")
		}
	} else if developmentUserID != "" {
		if err := cfg.DevelopmentUserID.Scan(developmentUserID); err != nil {
			return Config{}, fmt.Errorf("ECHONOTE_USER_ID must be a UUID")
		}
	}
	if cfg.PublicOrigin != "" {
		origin, parseErr := url.Parse(cfg.PublicOrigin)
		secureEnvironment := cfg.Environment == "staging" || cfg.Environment == "production"
		allowedScheme := origin.Scheme == "https" || (!secureEnvironment && origin.Scheme == "http")
		if parseErr != nil || !allowedScheme || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
			return Config{}, fmt.Errorf("PUBLIC_ORIGIN must be an HTTPS origin without credentials or a path")
		}
	}

	port, err := strconv.Atoi(envOrDefault("SERVER_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("SERVER_PORT must be an integer between 1 and 65535")
	}
	cfg.ServerPort = port
	cost, err := strconv.Atoi(envOrDefault("PASSWORD_BCRYPT_COST", "12"))
	if err != nil || cost < 10 || cost > 16 {
		return Config{}, fmt.Errorf("PASSWORD_BCRYPT_COST must be an integer between 10 and 16")
	}
	cfg.PasswordBcryptCost = cost

	if err := cfg.LogLevel.UnmarshalText([]byte(envOrDefault("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}

	if cfg.WorkerPollInterval, err = positiveDuration("WORKER_POLL_INTERVAL", cfg.WorkerPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = positiveDuration("SESSION_TTL", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.WorkerLeaseTimeout, err = positiveDuration("WORKER_LEASE_TIMEOUT", cfg.WorkerLeaseTimeout); err != nil {
		return Config{}, err
	}
	if cfg.ASRPollInterval, err = positiveDuration("ASR_POLL_INTERVAL", cfg.ASRPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.ASRProvider != "" && cfg.ASRProvider != "aliyun" {
		return Config{}, fmt.Errorf("ASR_PROVIDER must be aliyun")
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

	return cfg, nil
}

func (cfg Config) SecureCookies() bool {
	return cfg.Environment == "staging" || cfg.Environment == "production"
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
