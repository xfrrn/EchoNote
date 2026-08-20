package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/logging"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("migrate exited", "service", "echonote-migrate", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate up | migrate version | migrate down <steps>")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New("echonote-migrate", cfg.Environment, cfg.LogLevel)

	switch args[0] {
	case "up":
		if len(args) != 1 {
			return fmt.Errorf("usage: migrate up")
		}
		if err := database.MigrateUp(cfg.DatabaseURL); err != nil {
			return err
		}
		logger.Info("migrations applied")
		return nil
	case "version":
		if len(args) != 1 {
			return fmt.Errorf("usage: migrate version")
		}
		version, dirty, err := database.MigrationVersion(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		logger.Info("migration version", "version", version, "dirty", dirty)
		return nil
	case "down":
		if len(args) != 2 {
			return fmt.Errorf("usage: migrate down <steps>")
		}
		steps, err := strconv.Atoi(args[1])
		if err != nil || steps < 1 {
			return fmt.Errorf("down steps must be a positive integer")
		}
		if err := database.MigrateDown(cfg.DatabaseURL, steps); err != nil {
			return err
		}
		logger.Info("migrations rolled back", "steps", steps)
		return nil
	default:
		return fmt.Errorf("unknown migration command %q", args[0])
	}
}
