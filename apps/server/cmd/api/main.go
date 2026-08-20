package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	httpapi "github.com/Actify/echonote/apps/server/internal/http"
	"github.com/Actify/echonote/apps/server/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("api exited", "service", "echonote-api", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New("echonote-api", cfg.Environment, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	connectContext, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	pool, err := database.Open(connectContext, cfg.DatabaseURL, "echonote-api")
	cancelConnect()
	if err != nil {
		return err
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.ServerPort),
		Handler:           httpapi.NewRouter(pool, logger),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errorsChannel := make(chan error, 1)
	go func() {
		logger.Info("api started", "address", server.Addr)
		errorsChannel <- server.ListenAndServe()
	}()

	select {
	case err := <-errorsChannel:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		logger.Info("api stopped")
		return nil
	}
}
