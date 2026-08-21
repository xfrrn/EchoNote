package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/workerruntime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker exited", "service", "echonote-worker", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 || len(args) == 1 && args[0] != "--check-config" {
		return fmt.Errorf("usage: worker [--check-config]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.ValidateWorker(); err != nil {
		return fmt.Errorf("validate worker config: %w", err)
	}
	if len(args) == 1 {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.OpenApplication(ctx, cfg.DatabaseURL, "echonote-worker")
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.SecureEnvironment() {
		if err := database.ValidateRuntimeRole(ctx, pool, cfg.ExpectedDatabase); err != nil {
			return err
		}
	}

	return workerruntime.Run(ctx, cfg, pool)
}
