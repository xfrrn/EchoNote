package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/logging"
	podcastprovider "github.com/Actify/echonote/apps/server/internal/provider/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
	workerapp "github.com/Actify/echonote/apps/server/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker exited", "service", "echonote-worker", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New("echonote-worker", cfg.Environment, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectContext, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	pool, err := database.Open(connectContext, cfg.DatabaseURL, "echonote-worker")
	cancelConnect()
	if err != nil {
		return err
	}
	defer pool.Close()

	hostname, _ := os.Hostname()
	workerID := hostname + "-" + strconv.Itoa(os.Getpid())
	queue := repository.NewJobQueue(pool)
	imports := repository.NewImportRepository(pool)
	httpClient := podcastprovider.NewHTTPClient()
	defer httpClient.CloseIdleConnections()
	resolver := podcastprovider.NewResolver(httpClient)
	process := workerapp.New(
		queue,
		logger,
		workerID,
		cfg.WorkerPollInterval,
		cfg.WorkerLeaseTimeout,
		map[string]workerapp.Handler{
			repository.ResolveEpisodeJobType: service.NewResolveImportHandler(imports, resolver),
		},
	)
	return process.Run(ctx)
}
