package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	httpapi "github.com/Actify/echonote/apps/server/internal/http"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/workerruntime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("api exited", "service", "echonote-api", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 || len(args) == 1 && args[0] != "--check-config" {
		return fmt.Errorf("usage: api [--check-config]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.ValidateAPI(); err != nil {
		return fmt.Errorf("validate API config: %w", err)
	}
	if len(args) == 1 {
		return nil
	}
	logger := logging.New("echonote-api", cfg.Environment, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.OpenApplication(ctx, cfg.DatabaseURL, "echonote-api")
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.SecureEnvironment() {
		if err := database.ValidateRuntimeRole(ctx, pool, cfg.ExpectedDatabase); err != nil {
			return err
		}
	}
	server := &http.Server{
		Addr: cfg.ListenAddress(),
		Handler: httpapi.NewRouter(
			pool,
			repository.NewImportRepository(pool),
			repository.NewUserRepository(pool),
			cfg.InternalToken,
			logger,
		),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errorsChannel := make(chan error, 1)
	workerErrors := make(chan error, 1)
	if !cfg.SecureEnvironment() {
		go func() {
			workerErrors <- workerruntime.Run(ctx, cfg, pool)
		}()
	}
	go func() {
		logger.Info("api started", "address", server.Addr)
		errorsChannel <- server.ListenAndServe()
	}()

	var runErr error
	select {
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("serve HTTP: %w", err)
		}
	case err := <-workerErrors:
		if err != nil {
			runErr = fmt.Errorf("run embedded worker: %w", err)
		}
	case <-ctx.Done():
	}
	stop()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown HTTP server: %w", err))
	}
	logger.Info("api stopped")
	return runErr
}
