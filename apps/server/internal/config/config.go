package config

import (
	"fmt"
	"log/slog"
	"os"
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
	UserID             pgtype.UUID
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        envOrDefault("APP_ENV", "development"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		WorkerPollInterval: time.Second,
		WorkerLeaseTimeout: 5 * time.Minute,
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

	return cfg, nil
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
