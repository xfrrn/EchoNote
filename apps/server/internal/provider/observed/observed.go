package observed

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
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

func (provider *asr) Cancel(ctx context.Context, taskID string) error {
	started := time.Now()
	err := provider.next.Cancel(ctx, taskID)
	providerLog(ctx, provider.logger, "aliyun_asr", "cancel", started, err)
	return err
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

type embedding struct {
	next   searchdomain.EmbeddingProvider
	logger *slog.Logger
}

func WrapEmbedding(next searchdomain.EmbeddingProvider, logger *slog.Logger) searchdomain.EmbeddingProvider {
	return &embedding{next: next, logger: logger}
}

func (provider *embedding) Embed(ctx context.Context, texts []string, inputType searchdomain.EmbeddingInputType) ([][]float32, error) {
	started := time.Now()
	result, err := provider.next.Embed(ctx, texts, inputType)
	providerLog(ctx, provider.logger, "aliyun_embedding", "embed", started, err,
		"model", provider.next.Model(), "input_type", inputType, "input_count", len(texts))
	return result, err
}

func (provider *embedding) Model() string   { return provider.next.Model() }
func (provider *embedding) Dimensions() int { return provider.next.Dimensions() }

type llm struct {
	next   aidomain.LLMProvider
	logger *slog.Logger
}

func WrapLLM(next aidomain.LLMProvider, logger *slog.Logger) aidomain.LLMProvider {
	return &llm{next: next, logger: logger}
}

func (provider *llm) GenerateStructured(ctx context.Context, request aidomain.StructuredGenerationRequest) (aidomain.StructuredGenerationResult, error) {
	started := time.Now()
	result, err := provider.next.GenerateStructured(ctx, request)
	providerLog(ctx, provider.logger, "aliyun_llm", "generate_structured", started, err,
		"model", provider.next.Model(), "input_tokens", result.Usage.InputTokens, "output_tokens", result.Usage.OutputTokens)
	return result, err
}

func (provider *llm) StreamChat(ctx context.Context, request aidomain.ChatRequest) (<-chan aidomain.ChatEvent, error) {
	started := time.Now()
	events, err := provider.next.StreamChat(ctx, request)
	if err != nil {
		providerLog(ctx, provider.logger, "aliyun_llm", "stream_chat", started, err, "model", provider.next.Model())
		return nil, err
	}
	observedEvents := make(chan aidomain.ChatEvent)
	go func() {
		defer close(observedEvents)
		usage := aidomain.Usage{}
		var streamErr error
		for {
			select {
			case <-ctx.Done():
				providerLog(ctx, provider.logger, "aliyun_llm", "stream_chat", started, ctx.Err(),
					"model", provider.next.Model(), "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens)
				return
			case event, ok := <-events:
				if !ok {
					providerLog(ctx, provider.logger, "aliyun_llm", "stream_chat", started, streamErr,
						"model", provider.next.Model(), "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens)
					return
				}
				if event.Usage != nil {
					usage = *event.Usage
				}
				if event.Err != nil {
					streamErr = event.Err
				}
				select {
				case observedEvents <- event:
				case <-ctx.Done():
					providerLog(ctx, provider.logger, "aliyun_llm", "stream_chat", started, ctx.Err(),
						"model", provider.next.Model(), "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens)
					return
				}
			}
		}
	}()
	return observedEvents, nil
}

func (provider *llm) Model() string { return provider.next.Model() }

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
