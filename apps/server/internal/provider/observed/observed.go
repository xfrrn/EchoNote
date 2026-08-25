package observed

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	transcriptiondomain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/logging"
)

type asr struct {
	next   transcriptiondomain.ASRProvider
	logger *slog.Logger
}

func WrapASR(next transcriptiondomain.ASRProvider, logger *slog.Logger) transcriptiondomain.ASRProvider {
	return &asr{next: next, logger: logger}
}

func (provider *asr) Submit(ctx context.Context, request transcriptiondomain.Request) (transcriptiondomain.ExternalTask, error) {
	started := time.Now()
	result, err := provider.next.Submit(ctx, request)
	providerLog(ctx, provider.logger, "aliyun_asr", "submit", started, err,
		"model", request.Model, "audio_duration_ms", request.AudioDurationMS)
	return result, err
}

func (provider *asr) Poll(ctx context.Context, taskID string) (transcriptiondomain.ExternalTaskStatus, error) {
	started := time.Now()
	result, err := provider.next.Poll(ctx, taskID)
	providerLog(ctx, provider.logger, "aliyun_asr", "poll", started, err)
	return result, err
}

func (provider *asr) FetchResult(ctx context.Context, resultURL string) (transcriptiondomain.RawResult, error) {
	started := time.Now()
	result, err := provider.next.FetchResult(ctx, resultURL)
	providerLog(ctx, provider.logger, "aliyun_asr", "fetch_result", started, err)
	return result, err
}

type objectStore struct {
	next   transcriptiondomain.ObjectStore
	logger *slog.Logger
}

func WrapObjectStore(next transcriptiondomain.ObjectStore, logger *slog.Logger) transcriptiondomain.ObjectStore {
	return &objectStore{next: next, logger: logger}
}

func (store *objectStore) Put(ctx context.Context, key string, reader io.Reader) error {
	started := time.Now()
	err := store.next.Put(ctx, key, reader)
	providerLog(ctx, store.logger, "aliyun_oss", "put", started, err)
	return err
}

func (store *objectStore) Delete(ctx context.Context, key string) error {
	started := time.Now()
	err := store.next.Delete(ctx, key)
	providerLog(ctx, store.logger, "aliyun_oss", "delete", started, err)
	return err
}

func (store *objectStore) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	started := time.Now()
	result, err := store.next.SignedURL(ctx, key, ttl)
	providerLog(ctx, store.logger, "aliyun_oss", "signed_url", started, err)
	return result, err
}

func providerLog(ctx context.Context, fallback *slog.Logger, provider, operation string, started time.Time, err error, extra ...any) {
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	attributes := []any{
		"provider", provider,
		"operation", operation,
		"duration_ms", elapsedMilliseconds(started),
		"status", status,
	}
	attributes = append(attributes, extra...)
	logger := logging.FromContext(ctx, fallback)
	if err == nil {
		logger.InfoContext(ctx, "provider request", attributes...)
		return
	}
	errorCode := "PROVIDER_ERROR"
	var classified interface{ Code() string }
	if errors.As(err, &classified) && classified.Code() != "" {
		errorCode = classified.Code()
	}
	attributes = append(attributes, "error_code", errorCode)
	var response interface{ ProviderStatus() int }
	if errors.As(err, &response) && response.ProviderStatus() > 0 {
		attributes = append(attributes, "provider_status", response.ProviderStatus())
	}
	logger.WarnContext(ctx, "provider request", attributes...)
}

func elapsedMilliseconds(started time.Time) int64 {
	if elapsed := time.Since(started).Milliseconds(); elapsed > 0 {
		return elapsed
	}
	return 1
}
