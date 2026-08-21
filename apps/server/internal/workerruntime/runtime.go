package workerruntime

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
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
	"github.com/Actify/echonote/apps/server/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	logger := logging.New("echonote-worker", cfg.Environment, cfg.LogLevel)
	removed, err := service.CleanupTemporaryFiles(os.TempDir(), time.Now().Add(-cfg.WorkerTempMaxAge))
	if err != nil {
		return fmt.Errorf("cleanup worker temporary files: %w", err)
	}
	if removed > 0 {
		logger.Info("removed stale temporary files", "count", removed)
	}

	hostname, _ := os.Hostname()
	workerID := hostname + "-" + strconv.Itoa(os.Getpid())
	queue := repository.NewJobQueue(pool)
	imports := repository.NewImportRepository(pool)
	httpClient := podcastprovider.NewHTTPClient()
	defer httpClient.CloseIdleConnections()
	resolver := podcastprovider.NewResolver(httpClient)
	handlers := map[string]worker.Handler{
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
			repository.NewTranscriptionRepository(pool, cfg.ASRStandardModel, cfg.ASRQualityModel), audio.NewDownloader(), audioProcessor,
			objectStore, asrProvider, cfg.ASRPollInterval,
		)
		for jobType, handler := range workflow.Handlers() {
			handlers[jobType] = handler
		}
	} else {
		logger.Warn("transcription worker disabled", "reason", cfg.ValidateTranscription())
	}
	return worker.New(
		queue,
		logger,
		workerID,
		cfg.WorkerPollInterval,
		cfg.WorkerLeaseTimeout,
		handlers,
	).Run(ctx)
}
