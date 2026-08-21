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
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	httpapi "github.com/Actify/echonote/apps/server/internal/http"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/provider/embedding"
	llmprovider "github.com/Actify/echonote/apps/server/internal/provider/llm"
	"github.com/Actify/echonote/apps/server/internal/provider/observed"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
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
	authRepository := repository.NewAuthRepository(pool)
	if err := authRepository.EnsurePlaceholderUser(ctx, cfg.DevelopmentUserID); err != nil {
		return err
	}
	var embeddingProvider searchdomain.EmbeddingProvider
	if cfg.EmbeddingEnabled() {
		embeddingProvider, err = embedding.NewAliyun(cfg.EmbeddingEndpoint, cfg.EmbeddingAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure embedding: %w", err)
		}
		embeddingProvider = observed.WrapEmbedding(embeddingProvider, logger)
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
		llmProvider = observed.WrapLLM(llmProvider, logger)
	} else {
		logger.Warn("AI generation disabled", "reason", cfg.ValidateLLM())
	}
	aiService := service.NewAIService(repository.NewAIRepository(pool), searchService, llmProvider)
	exportService := service.NewExportService(repository.NewExportRepository(pool))

	server := &http.Server{
		Addr: cfg.ListenAddress(),
		Handler: httpapi.NewRouter(
			pool,
			repository.NewImportRepository(pool),
			repository.NewLibraryRepository(pool),
			repository.NewNotesRepository(pool),
			repository.NewTranscriptionRepository(pool),
			searchService,
			aiService,
			exportService,
			authRepository,
			cfg.TranscriptionAPI,
			httpapi.AuthConfig{
				PublicOrigin: cfg.PublicOrigin, SessionTTL: cfg.SessionTTL, PasswordCost: cfg.PasswordBcryptCost,
				SecureCookies: cfg.SecureCookies(), DevelopmentUserID: cfg.DevelopmentUserID,
			},
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
