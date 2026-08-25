package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
	workerapp "github.com/Actify/echonote/apps/server/internal/worker"
	"github.com/jackc/pgx/v5/pgtype"
)

const signedURLTTL = 4 * time.Hour

type AudioDownloader interface {
	Download(context.Context, string, string) (string, error)
}

type AudioProcessor interface {
	Prepare(context.Context, string, string) (int64, string, error)
	Render(context.Context, string, int64, int64, string) (string, error)
}

type TranscriptionWorkflow struct {
	repository   *repository.TranscriptionRepository
	downloader   AudioDownloader
	processor    AudioProcessor
	store        domain.ObjectStore
	asr          domain.ASRProvider
	pollInterval time.Duration
}

func NewTranscriptionWorkflow(
	repository *repository.TranscriptionRepository,
	downloader AudioDownloader,
	processor AudioProcessor,
	store domain.ObjectStore,
	asr domain.ASRProvider,
	pollInterval time.Duration,
) *TranscriptionWorkflow {
	return &TranscriptionWorkflow{
		repository: repository, downloader: downloader, processor: processor,
		store: store, asr: asr, pollInterval: pollInterval,
	}
}

func (workflow *TranscriptionWorkflow) Handlers() map[string]workerapp.Handler {
	return map[string]workerapp.Handler{
		repository.DownloadAudioJobType:     workflow.wrap(workflow.download),
		repository.PrepareAudioJobType:      workflow.wrap(workflow.prepare),
		repository.PlanTranscriptionJobType: workflow.wrap(workflow.plan),
		repository.RenderAudioChunkJobType:  workflow.wrap(workflow.render),
		repository.SubmitASRJobType:         workflow.wrap(workflow.submit),
		repository.PollASRJobType:           workflow.wrap(workflow.poll),
		repository.IngestASRResultJobType:   workflow.wrap(workflow.ingest),
		repository.AlignSpeakersJobType:     workflow.wrap(workflow.align),
		repository.MergeTranscriptJobType:   workflow.wrap(workflow.merge),
		repository.CleanupAudioJobType:      workflow.wrap(workflow.cleanup),
	}
}

func (workflow *TranscriptionWorkflow) wrap(handler workerapp.Handler) workerapp.Handler {
	return func(ctx context.Context, job db.Job) error {
		err := handler(ctx, job)
		if err == nil {
			return nil
		}
		var delayed interface{ RescheduleAfter() time.Duration }
		if errors.As(err, &delayed) {
			return err
		}
		retryable := true
		var classified interface{ Retryable() bool }
		if errors.As(err, &classified) {
			retryable = classified.Retryable()
		}
		if !retryable || job.Attempt >= job.MaxAttempts {
			if recordErr := workflow.repository.FailJob(ctx, job, err); recordErr != nil {
				return errors.Join(err, recordErr)
			}
		}
		return err
	}
}

func (workflow *TranscriptionWorkflow) download(ctx context.Context, job db.Job) error {
	run, shouldRun, err := workflow.repository.BeginDownload(ctx, job.EntityID)
	if err != nil || !shouldRun {
		return err
	}
	if strings.TrimSpace(run.AudioUrl) == "" {
		return permanent("AUDIO_SOURCE_MISSING", "episode has no audio source", nil)
	}
	filePath, cleanup, err := temporaryFile("echonote-source-*")
	if err != nil {
		return err
	}
	defer cleanup()
	hash, err := workflow.downloader.Download(ctx, run.AudioUrl, filePath)
	if err != nil {
		return err
	}
	key := objectPrefix(run.UserID, run.EpisodeID, run.ID) + "/source/original"
	if err := putFile(ctx, workflow.store, key, filePath); err != nil {
		return err
	}
	return workflow.repository.FinishDownload(ctx, run.ID, key, hash)
}

func (workflow *TranscriptionWorkflow) prepare(ctx context.Context, job db.Job) error {
	run, err := workflow.repository.GetRunForJob(ctx, job.EntityID)
	if err != nil {
		return err
	}
	if run.Status != "preparing" || run.PreparedObjectKey != nil {
		return nil
	}
	if run.SourceObjectKey == nil {
		return permanent("AUDIO_SOURCE_OBJECT_MISSING", "downloaded audio object is missing", nil)
	}
	sourceURL, err := workflow.store.SignedURL(ctx, *run.SourceObjectKey, signedURLTTL)
	if err != nil {
		return err
	}
	filePath, cleanup, err := temporaryFile("echonote-prepared-*.flac")
	if err != nil {
		return err
	}
	defer cleanup()
	durationMS, hash, err := workflow.processor.Prepare(ctx, sourceURL, filePath)
	if err != nil {
		return err
	}
	key := objectPrefix(run.UserID, run.EpisodeID, run.ID) + "/source/prepared.flac"
	if err := putFile(ctx, workflow.store, key, filePath); err != nil {
		return err
	}
	return workflow.repository.FinishPrepare(ctx, run.ID, key, hash, durationMS)
}

func (workflow *TranscriptionWorkflow) plan(ctx context.Context, job db.Job) error {
	run, err := workflow.repository.GetRunForJob(ctx, job.EntityID)
	if err != nil {
		return err
	}
	if run.Status != "preparing" || run.TotalChunks > 0 {
		return nil
	}
	if run.DurationMs == nil {
		return permanent("AUDIO_DURATION_MISSING", "prepared audio duration is missing", nil)
	}
	windows, err := domain.Plan(*run.DurationMs)
	if err != nil {
		return permanent("AUDIO_DURATION_INVALID", err.Error(), err)
	}
	return workflow.repository.Plan(ctx, run.ID, windows)
}

func (workflow *TranscriptionWorkflow) render(ctx context.Context, job db.Job) error {
	chunk, shouldRun, err := workflow.repository.BeginRender(ctx, job.EntityID)
	if err != nil || !shouldRun {
		return err
	}
	if chunk.PreparedObjectKey == nil {
		return permanent("PREPARED_AUDIO_MISSING", "prepared audio object is missing", nil)
	}
	ctx = transcriptionLogContext(ctx, chunk.EpisodeID, chunk.TranscriptionRunID)
	sourceURL, err := workflow.store.SignedURL(ctx, *chunk.PreparedObjectKey, signedURLTTL)
	if err != nil {
		return err
	}
	filePath, cleanup, err := temporaryFile("echonote-chunk-*.flac")
	if err != nil {
		return err
	}
	defer cleanup()
	hash, err := workflow.processor.Render(ctx, sourceURL, chunk.RenderStartMs, chunk.RenderEndMs, filePath)
	if err != nil {
		return err
	}
	key := objectPrefix(chunk.UserID, chunk.EpisodeID, chunk.TranscriptionRunID) + "/chunks/" + fmt.Sprintf("%04d.flac", chunk.Sequence)
	if err := putFile(ctx, workflow.store, key, filePath); err != nil {
		return err
	}
	fingerprint := chunkFingerprint(hash, chunk, string(chunk.Config))
	return workflow.repository.FinishRender(ctx, chunk.ID, key, hash, fingerprint)
}

func (workflow *TranscriptionWorkflow) submit(ctx context.Context, job db.Job) error {
	chunk, shouldRun, err := workflow.repository.BeginSubmit(ctx, job.EntityID)
	if errors.Is(err, repository.ErrSubmissionAmbiguous) {
		return permanent("ASR_SUBMISSION_AMBIGUOUS", err.Error(), err)
	}
	if err != nil || !shouldRun {
		return err
	}
	if chunk.ObjectKey == nil {
		return permanent("CHUNK_OBJECT_MISSING", "rendered chunk object is missing", nil)
	}
	ctx = transcriptionLogContext(ctx, chunk.EpisodeID, chunk.TranscriptionRunID)
	audioURL, err := workflow.store.SignedURL(ctx, *chunk.ObjectKey, signedURLTTL)
	if err != nil {
		_ = workflow.repository.ResetSubmit(ctx, chunk.ID)
		return err
	}
	config, err := decodeRunConfig(chunk.Config)
	if err != nil {
		_ = workflow.repository.ResetSubmit(ctx, chunk.ID)
		return permanent("TRANSCRIPTION_CONFIG_INVALID", err.Error(), err)
	}
	task, err := workflow.asr.Submit(ctx, domain.Request{
		AudioURL: audioURL, AudioDurationMS: chunk.RenderEndMs - chunk.RenderStartMs,
		Model: chunk.Model, LanguageHint: config.LanguageHint, SpeakerCount: config.SpeakerCount,
	})
	if err != nil {
		var ambiguous interface{ AmbiguousCost() bool }
		if errors.As(err, &ambiguous) && ambiguous.AmbiguousCost() {
			return permanent("ASR_SUBMISSION_AMBIGUOUS", err.Error(), err)
		}
		_ = workflow.repository.ResetSubmit(ctx, chunk.ID)
		return err
	}
	return workflow.repository.FinishSubmit(ctx, chunk.ID, task.ID, workflow.pollInterval)
}

func (workflow *TranscriptionWorkflow) poll(ctx context.Context, job db.Job) error {
	chunk, err := workflow.repository.GetChunkForJob(ctx, job.EntityID)
	if err != nil {
		return err
	}
	if chunk.RunStatus != "transcribing" || (chunk.Status != "submitted" && chunk.Status != "running") || chunk.ResultUrl != nil {
		return nil
	}
	if chunk.ExternalTaskID == nil {
		return permanent("ASR_TASK_ID_MISSING", "chunk has no external ASR task ID", nil)
	}
	ctx = transcriptionLogContext(ctx, chunk.EpisodeID, chunk.TranscriptionRunID)
	status, err := workflow.asr.Poll(ctx, *chunk.ExternalTaskID)
	if err != nil {
		return err
	}
	switch status.State {
	case domain.TaskPending, domain.TaskRunning:
		if err := workflow.repository.MarkPolling(ctx, chunk.ID); err != nil {
			return err
		}
		return workerapp.RescheduleAfter(workflow.pollInterval)
	case domain.TaskSucceeded:
		if status.ResultURL == "" {
			return permanent("ASR_RESULT_URL_MISSING", "ASR task succeeded without a result URL", nil)
		}
		return workflow.repository.FinishPoll(ctx, chunk.ID, status.ResultURL)
	case domain.TaskFailed:
		if err := workflow.repository.ClearExternalTask(ctx, chunk.ID); err != nil {
			return err
		}
		return permanent(valueOr(status.Code, "ASR_TASK_FAILED"), valueOr(status.Message, "ASR task failed"), nil)
	case domain.TaskCanceled:
		if err := workflow.repository.ClearExternalTask(ctx, chunk.ID); err != nil {
			return err
		}
		return permanent("ASR_TASK_CANCELED", "ASR task was canceled", nil)
	default:
		return permanent("ASR_TASK_STATUS_INVALID", "ASR returned an invalid task state", nil)
	}
}

func (workflow *TranscriptionWorkflow) ingest(ctx context.Context, job db.Job) error {
	chunk, err := workflow.repository.GetChunkForJob(ctx, job.EntityID)
	if err != nil {
		return err
	}
	if chunk.Status == "completed" || chunk.RunStatus != "transcribing" {
		return nil
	}
	if chunk.Status != "running" || chunk.ResultUrl == nil || chunk.DurationMs == nil {
		return permanent("ASR_RESULT_NOT_READY", "chunk result is not ready for ingestion", nil)
	}
	ctx = transcriptionLogContext(ctx, chunk.EpisodeID, chunk.TranscriptionRunID)
	result, err := workflow.asr.FetchResult(ctx, *chunk.ResultUrl)
	if err != nil {
		var classified interface{ Retryable() bool }
		if errors.As(err, &classified) && !classified.Retryable() {
			if clearErr := workflow.repository.ClearExternalTask(ctx, chunk.ID); clearErr != nil {
				return clearErr
			}
		}
		return err
	}
	normalized := domain.ToEpisodeTime(result, chunk.RenderStartMs, *chunk.DurationMs)
	if len(normalized.Segments) == 0 {
		return permanent("ASR_RESULT_EMPTY", "ASR result contains no valid segments", nil)
	}
	return workflow.repository.FinishIngest(ctx, chunk.ID, normalized)
}

func (workflow *TranscriptionWorkflow) align(ctx context.Context, job db.Job) error {
	run, chunks, shouldRun, err := workflow.repository.BeginAlignment(ctx, job.EntityID)
	if err != nil || !shouldRun {
		return err
	}
	domainChunks, err := decodeChunks(chunks)
	if err != nil {
		return permanent("NORMALIZED_TRANSCRIPT_INVALID", err.Error(), err)
	}
	alignment, err := domain.Align(domainChunks)
	if err != nil {
		return permanent("SPEAKER_ALIGNMENT_FAILED", err.Error(), err)
	}
	return workflow.repository.FinishAlignment(ctx, run, chunks, alignment)
}

func (workflow *TranscriptionWorkflow) merge(ctx context.Context, job db.Job) error {
	run, err := workflow.repository.GetRunForJob(ctx, job.EntityID)
	if err != nil {
		return err
	}
	if run.Status != "merging" {
		return nil
	}
	chunks, err := workflow.repository.Chunks(ctx, run.ID)
	if err != nil {
		return err
	}
	domainChunks, err := decodeChunks(chunks)
	if err != nil {
		return permanent("NORMALIZED_TRANSCRIPT_INVALID", err.Error(), err)
	}
	segments, err := domain.Merge(domainChunks)
	if err != nil {
		return permanent("TRANSCRIPT_MERGE_FAILED", err.Error(), err)
	}
	if len(segments) == 0 {
		return permanent("TRANSCRIPT_EMPTY", "merged transcript contains no segments", nil)
	}
	_, err = workflow.repository.ActivateTranscript(ctx, run.ID, domain.SpeakersFromChunks(domainChunks), segments)
	return err
}

func (workflow *TranscriptionWorkflow) cleanup(ctx context.Context, job db.Job) error {
	var payload struct {
		Scope string   `json:"scope"`
		Keys  []string `json:"keys,omitempty"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return permanent("CLEANUP_PAYLOAD_INVALID", "cleanup payload is invalid", err)
	}
	if payload.Scope == "deleted_episode" || payload.Scope == "objects" {
		for _, key := range payload.Keys {
			if err := workflow.store.Delete(ctx, key); err != nil {
				return err
			}
		}
		return nil
	}
	return workflow.cleanupScope(ctx, job.EntityID, payload.Scope)
}

func (workflow *TranscriptionWorkflow) cleanupScope(ctx context.Context, runID pgtype.UUID, scope string) error {
	objects, err := workflow.repository.CleanupObjects(ctx, runID)
	if err != nil {
		return err
	}
	keys := objects.AudioKeys
	if scope == "chunks" {
		keys = objects.ChunkKeys
	} else if scope != "audio" {
		return permanent("CLEANUP_SCOPE_INVALID", "cleanup scope is invalid", nil)
	}
	for _, key := range keys {
		if err := workflow.store.Delete(ctx, key); err != nil {
			return err
		}
	}
	return workflow.repository.MarkCleaned(ctx, runID, scope)
}

func decodeChunks(chunks []db.TranscriptionChunk) ([]domain.Chunk, error) {
	result := make([]domain.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.NormalizedResult) == 0 {
			return nil, fmt.Errorf("chunk %d has no normalized result", chunk.Sequence)
		}
		var normalized domain.Result
		if err := json.Unmarshal(chunk.NormalizedResult, &normalized); err != nil {
			return nil, fmt.Errorf("chunk %d result: %w", chunk.Sequence, err)
		}
		mapping := map[string]string(nil)
		if len(chunk.SpeakerMap) > 0 {
			if err := json.Unmarshal(chunk.SpeakerMap, &mapping); err != nil {
				return nil, fmt.Errorf("chunk %d speaker map: %w", chunk.Sequence, err)
			}
		}
		result = append(result, domain.Chunk{
			ID: chunk.ID.String(),
			Window: domain.Window{
				Sequence: int(chunk.Sequence), CoreStartMS: chunk.CoreStartMs, CoreEndMS: chunk.CoreEndMs,
				RenderStartMS: chunk.RenderStartMs, RenderEndMS: chunk.RenderEndMs,
			},
			Segments: normalized.Segments, SpeakerMap: mapping,
		})
	}
	return result, nil
}

func decodeRunConfig(raw []byte) (repository.RunConfig, error) {
	var config repository.RunConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return repository.RunConfig{}, err
	}
	return config, nil
}

func objectPrefix(userID, episodeID, runID pgtype.UUID) string {
	return "users/" + userID.String() + "/episodes/" + episodeID.String() + "/transcription-runs/" + runID.String()
}

func transcriptionLogContext(ctx context.Context, episodeID, runID pgtype.UUID) context.Context {
	return logging.WithAttributes(ctx, "episode_id", episodeID.String(), "transcription_run_id", runID.String())
}

func chunkFingerprint(audioHash string, chunk db.GetTranscriptionChunkForJobRow, config string) string {
	payload := strings.Join([]string{
		audioHash, strconv.FormatInt(chunk.RenderStartMs, 10), strconv.FormatInt(chunk.RenderEndMs, 10),
		chunk.Provider, chunk.Model, config,
	}, "|")
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func temporaryFile(pattern string) (string, func(), error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}
	return name, func() { _ = os.Remove(name) }, nil
}

func CleanupTemporaryFiles(directory string, olderThan time.Time) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	removed := 0
	var result error
	// ponytail: the age fence assumes one worker host; use per-worker directories before horizontal scaling.
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "echonote-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if !info.ModTime().Before(olderThan) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			result = errors.Join(result, err)
			continue
		}
		removed++
	}
	return removed, result
}

func putFile(ctx context.Context, store domain.ObjectStore, key, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return store.Put(ctx, key, file)
}

type jobError struct {
	code, message string
	cause         error
	retryable     bool
}

func (err *jobError) Error() string   { return err.message }
func (err *jobError) Unwrap() error   { return err.cause }
func (err *jobError) Code() string    { return err.code }
func (err *jobError) Retryable() bool { return err.retryable }

func permanent(code, message string, cause error) error {
	return &jobError{code: code, message: message, cause: cause}
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
