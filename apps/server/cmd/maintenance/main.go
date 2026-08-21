package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("maintenance exited", "service", "echonote-maintenance", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 || args[0] != "retention" || args[1] != "--dry-run" && args[1] != "--apply-system" {
		return fmt.Errorf("usage: maintenance retention --dry-run | --apply-system")
	}
	apply := args[1] == "--apply-system"
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New("echonote-maintenance", cfg.Environment, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	connectContext, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	pool, err := database.Open(connectContext, cfg.DatabaseURL, "echonote-maintenance")
	cancelConnect()
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.SecureEnvironment() {
		if err := database.ValidateRuntimeRole(ctx, pool, cfg.ExpectedDatabase); err != nil {
			return err
		}
	}
	report, err := repository.NewMaintenanceRepository(pool).Retain(ctx, time.Now().UTC(), apply)
	if err != nil {
		return err
	}
	logger.Info("retention complete",
		"scope", "system", "dry_run", !apply,
		"sessions", report.Sessions,
		"completed_jobs", report.CompletedJobs,
		"failed_jobs", report.FailedJobs,
		"job_events", report.JobEvents,
	)
	return nil
}
