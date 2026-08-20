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
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	httpapi "github.com/Actify/echonote/apps/server/internal/http"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/provider/embedding"
	llmprovider "github.com/Actify/echonote/apps/server/internal/provider/llm"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
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
	var embeddingProvider searchdomain.EmbeddingProvider
	if cfg.EmbeddingEnabled() {
		embeddingProvider, err = embedding.NewAliyun(cfg.EmbeddingEndpoint, cfg.EmbeddingAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure embedding: %w", err)
		}
	} else {
		logger.Warn("semantic search disabled", "reason", cfg.ValidateEmbedding())
	}
	searchRepository := repository.NewSearchRepository(pool)
	searchService := service.NewSearchService(searchRepository, embeddingProvider)
	var llmProvider aidomain.LLMProvider
	if cfg.LLMEnabled() {
		llmProvider, err = llmprovider.NewAliyun(cfg.LLMEndpoint, cfg.LLMModel, cfg.LLMAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure LLM: %w", err)
		}
	} else {
		logger.Warn("AI generation disabled", "reason", cfg.ValidateLLM())
	}
	aiService := service.NewAIService(repository.NewAIRepository(pool), searchService, llmProvider)
	exportService := service.NewExportService(repository.NewExportRepository(pool))

	server := &http.Server{
		Addr: ":" + strconv.Itoa(cfg.ServerPort),
		Handler: httpapi.NewRouter(
			pool,
			repository.NewImportRepository(pool),
			repository.NewLibraryRepository(pool),
			repository.NewNotesRepository(pool),
			repository.NewTranscriptionRepository(pool),
			searchService,
			aiService,
			exportService,
			cfg.TranscriptionEnabled(),
			cfg.UserID,
			logger,
		),
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
