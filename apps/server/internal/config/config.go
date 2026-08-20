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

const defaultUserID = "00000000-0000-4000-8000-000000000001"

type Config struct {
	Environment        string
	ServerPort         int
	DatabaseURL        string
	LogLevel           slog.Level
	WorkerPollInterval time.Duration
	WorkerLeaseTimeout time.Duration
	ASRPollInterval    time.Duration
	ASRProvider        string
	ASRAPIKey          string
	ASREndpoint        string
	StorageProvider    string
	StorageRegion      string
	StorageEndpoint    string
	StorageBucket      string
	StorageAccessKey   string
	StorageSecretKey   string
	FFmpegPath         string
	FFprobePath        string
	UserID             pgtype.UUID
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        envOrDefault("APP_ENV", "development"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		WorkerPollInterval: time.Second,
		WorkerLeaseTimeout: 5 * time.Minute,
		ASRPollInterval:    5 * time.Second,
		ASRProvider:        strings.ToLower(strings.TrimSpace(os.Getenv("ASR_PROVIDER"))),
		ASRAPIKey:          strings.TrimSpace(os.Getenv("ASR_API_KEY")),
		ASREndpoint:        envOrDefault("ASR_ENDPOINT", "https://dashscope.aliyuncs.com"),
		StorageProvider:    strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))),
		StorageRegion:      strings.TrimSpace(os.Getenv("STORAGE_REGION")),
		StorageEndpoint:    strings.TrimSpace(os.Getenv("STORAGE_ENDPOINT")),
		StorageBucket:      strings.TrimSpace(os.Getenv("STORAGE_BUCKET")),
		StorageAccessKey:   strings.TrimSpace(os.Getenv("STORAGE_ACCESS_KEY")),
		StorageSecretKey:   strings.TrimSpace(os.Getenv("STORAGE_SECRET_KEY")),
		FFmpegPath:         envOrDefault("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:        envOrDefault("FFPROBE_PATH", "ffprobe"),
	}

	if cfg.Environment != "development" && cfg.Environment != "production" && cfg.Environment != "test" {
		return Config{}, fmt.Errorf("APP_ENV must be development, production, or test")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if err := cfg.UserID.Scan(envOrDefault("ECHONOTE_USER_ID", defaultUserID)); err != nil {
		return Config{}, fmt.Errorf("ECHONOTE_USER_ID must be a UUID")
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
	if cfg.ASRPollInterval, err = positiveDuration("ASR_POLL_INTERVAL", cfg.ASRPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.ASRProvider != "" && cfg.ASRProvider != "aliyun" {
		return Config{}, fmt.Errorf("ASR_PROVIDER must be aliyun")
	}
	if cfg.StorageProvider != "" && cfg.StorageProvider != "aliyun_oss" {
		return Config{}, fmt.Errorf("STORAGE_PROVIDER must be aliyun_oss")
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

	return cfg, nil
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
