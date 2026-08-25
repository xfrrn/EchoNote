package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("admin exited", "service", "echonote-admin", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 || args[0] != "retry-cleanup" {
		return fmt.Errorf("usage: admin retry-cleanup <job-id>")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL, "echonote-admin")
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.SecureEnvironment() {
		if err := database.ValidateRuntimeRole(ctx, pool, cfg.ExpectedDatabase); err != nil {
			return err
		}
	}
	var jobID pgtype.UUID
	if err := jobID.Scan(args[1]); err != nil {
		return fmt.Errorf("job ID must be a UUID")
	}
	job, err := repository.NewJobQueue(pool).RetryFailedCleanup(ctx, jobID)
	if err != nil {
		return err
	}
	logging.New("echonote-admin", cfg.Environment, cfg.LogLevel).Info("cleanup job queued for manual retry", "job_id", formatUUID(job.ID))
	return nil
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	value := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
