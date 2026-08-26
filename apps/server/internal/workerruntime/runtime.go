package workerruntime

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/provider/asr"
	"github.com/Actify/echonote/apps/server/internal/provider/audio"
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
	transcriptions := repository.NewTranscriptionRepository(pool, cfg.ASRQualityModel)
	httpClient := podcastprovider.NewHTTPClient()
	defer httpClient.CloseIdleConnections()
	resolver := podcastprovider.NewResolver(httpClient, cfg.SnapAnyAPIKey)
	handlers := map[string]worker.Handler{
		repository.ResolveEpisodeJobType: service.NewResolveImportHandler(imports, transcriptions, resolver),
	}
	if err := cfg.ValidateTranscription(); err != nil {
		return err
	}
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
		transcriptions, audio.NewDownloader(), audioProcessor, objectStore, asrProvider, cfg.ASRPollInterval,
	)
	for jobType, handler := range workflow.Handlers() {
		handlers[jobType] = handler
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
