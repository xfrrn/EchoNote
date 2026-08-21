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
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/provider/asr"
	"github.com/Actify/echonote/apps/server/internal/provider/audio"
	"github.com/Actify/echonote/apps/server/internal/provider/embedding"
	llmprovider "github.com/Actify/echonote/apps/server/internal/provider/llm"
	"github.com/Actify/echonote/apps/server/internal/provider/observed"
	podcastprovider "github.com/Actify/echonote/apps/server/internal/provider/podcast"
	"github.com/Actify/echonote/apps/server/internal/provider/storage"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
	workerapp "github.com/Actify/echonote/apps/server/internal/worker"
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
	logger := logging.New("echonote-worker", cfg.Environment, cfg.LogLevel)
	removed, err := service.CleanupTemporaryFiles(os.TempDir(), time.Now().Add(-cfg.WorkerTempMaxAge))
	if err != nil {
		return fmt.Errorf("cleanup worker temporary files: %w", err)
	}
	if removed > 0 {
		logger.Info("removed stale temporary files", "count", removed)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectContext, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	pool, err := database.Open(connectContext, cfg.DatabaseURL, "echonote-worker")
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

	hostname, _ := os.Hostname()
	workerID := hostname + "-" + strconv.Itoa(os.Getpid())
	queue := repository.NewJobQueue(pool)
	imports := repository.NewImportRepository(pool)
	httpClient := podcastprovider.NewHTTPClient()
	defer httpClient.CloseIdleConnections()
	resolver := podcastprovider.NewResolver(httpClient)
	handlers := map[string]workerapp.Handler{
		repository.ResolveEpisodeJobType: service.NewResolveImportHandler(imports, resolver),
	}
	var embeddingProvider searchdomain.EmbeddingProvider
	if cfg.EmbeddingEnabled() {
		embeddingProvider, err = embedding.NewAliyun(cfg.EmbeddingEndpoint, cfg.EmbeddingAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure embedding: %w", err)
		}
		embeddingProvider = observed.WrapEmbedding(embeddingProvider, logger)
	} else {
		logger.Warn("semantic indexing disabled", "reason", cfg.ValidateEmbedding())
	}
	for jobType, handler := range service.NewSearchWorkflow(repository.NewSearchRepository(pool), embeddingProvider).Handlers() {
		handlers[jobType] = handler
	}
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
	for jobType, handler := range service.NewAIWorkflow(repository.NewAIRepository(pool), llmProvider).Handlers() {
		handlers[jobType] = handler
	}
	if cfg.TranscriptionEnabled() {
		rawObjectStore, err := storage.NewAliyunOSS(storage.AliyunOSSConfig{
			Region: cfg.StorageRegion, Endpoint: cfg.StorageEndpoint, Bucket: cfg.StorageBucket,
			AccessKey: cfg.StorageAccessKey, SecretKey: cfg.StorageSecretKey,
		})
		if err != nil {
			return fmt.Errorf("configure object storage: %w", err)
		}
		objectStore := observed.WrapObjectStore(rawObjectStore, logger)
		rawASRProvider, err := asr.NewAliyun(cfg.ASREndpoint, cfg.ASRAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure ASR: %w", err)
		}
		asrProvider := observed.WrapASR(rawASRProvider, logger)
		audioProcessor, err := audio.NewFFmpeg(cfg.FFmpegPath, cfg.FFprobePath)
		if err != nil {
			return fmt.Errorf("configure audio processor: %w", err)
		}
		workflow := service.NewTranscriptionWorkflow(
			repository.NewTranscriptionRepository(pool), audio.NewDownloader(), audioProcessor,
			objectStore, asrProvider, cfg.ASRPollInterval,
		)
		for jobType, handler := range workflow.Handlers() {
			handlers[jobType] = handler
		}
	} else {
		logger.Warn("transcription worker disabled", "reason", cfg.ValidateTranscription())
	}
	process := workerapp.New(
		queue,
		logger,
		workerID,
		cfg.WorkerPollInterval,
		cfg.WorkerLeaseTimeout,
		handlers,
	)
	return process.Run(ctx)
}
