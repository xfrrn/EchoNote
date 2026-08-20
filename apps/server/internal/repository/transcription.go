package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DownloadAudioJobType     = "download_audio"
	PrepareAudioJobType      = "prepare_audio"
	PlanTranscriptionJobType = "plan_transcription"
	RenderAudioChunkJobType  = "render_audio_chunk"
	SubmitASRJobType         = "submit_asr"
	PollASRJobType           = "poll_asr"
	IngestASRResultJobType   = "ingest_asr_result"
	AlignSpeakersJobType     = "align_speakers"
	MergeTranscriptJobType   = "merge_transcript"
	CancelASRJobType         = "cancel_asr"
	CleanupAudioJobType      = "cleanup_audio"
	TranscriptionRunEntity   = "transcription_run"
	TranscriptionChunkEntity = "transcription_chunk"
	chunkRetention           = 72 * time.Hour
)

var (
	ErrEpisodeNotReady           = errors.New("episode is not ready for transcription")
	ErrTranscriptionRunning      = errors.New("episode already has an active transcription run")
	ErrTranscriptionNotRetryable = errors.New("transcription run is not failed")
	ErrSubmissionAmbiguous       = errors.New("ASR submission outcome is ambiguous")
)

type RunConfig struct {
	LanguageHint string `json:"language_hint,omitempty"`
	SpeakerCount int    `json:"speaker_count,omitempty"`
}

type TranscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewTranscriptionRepository(pool *pgxpool.Pool) *TranscriptionRepository {
	return &TranscriptionRepository{pool: pool}
}

func (r *TranscriptionRepository) Create(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	profile string,
	config RunConfig,
) (db.TranscriptionRun, error) {
	model := ""
	switch profile {
	case "economy":
		model = "paraformer-v2"
	case "quality":
		model = "fun-asr"
	default:
		return db.TranscriptionRun{}, errors.New("profile must be economy or quality")
	}
	if config.SpeakerCount != 0 && (config.SpeakerCount < 2 || config.SpeakerCount > 100) {
		return db.TranscriptionRun{}, errors.New("speaker count must be 2-100")
	}
	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return db.TranscriptionRun{}, err
	}
	run, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		episode, err := queries.LockEpisodeForTranscription(ctx, db.LockEpisodeForTranscriptionParams{EpisodeID: episodeID, UserID: userID})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if episode.ResolveStatus != "completed" || strings.TrimSpace(episode.AudioUrl) == "" {
			return db.TranscriptionRun{}, ErrEpisodeNotReady
		}
		run, err := queries.CreateTranscriptionRun(ctx, db.CreateTranscriptionRunParams{
			UserID: userID, EpisodeID: episodeID, Profile: profile, Provider: "aliyun", Model: model, Config: encodedConfig,
		})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: "queued", EpisodeID: episodeID, UserID: userID}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, DownloadAudioJobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "queued", map[string]any{"profile": profile, "model": model}); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	if activeTranscriptionConflict(err) {
		return db.TranscriptionRun{}, ErrTranscriptionRunning
	}
	if err != nil {
		return db.TranscriptionRun{}, fmt.Errorf("create transcription: %w", err)
	}
	return run, nil
}

func (r *TranscriptionRepository) Get(ctx context.Context, userID, runID pgtype.UUID) (db.TranscriptionRun, error) {
	run, err := db.New(r.pool).GetTranscriptionRun(ctx, db.GetTranscriptionRunParams{RunID: runID, UserID: userID})
	if err != nil {
		return db.TranscriptionRun{}, fmt.Errorf("get transcription: %w", err)
	}
	return run, nil
}

func (r *TranscriptionRepository) Events(ctx context.Context, userID, runID pgtype.UUID, afterID int64) ([]db.TranscriptionEvent, error) {
	events, err := db.New(r.pool).ListTranscriptionEventsAfter(ctx, db.ListTranscriptionEventsAfterParams{
		RunID: runID, UserID: userID, AfterID: afterID, PageLimit: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("list transcription events: %w", err)
	}
	return events, nil
}

func (r *TranscriptionRepository) BeginDownload(ctx context.Context, runID pgtype.UUID) (db.GetTranscriptionRunForJobRow, bool, error) {
	type result struct {
		run    db.GetTranscriptionRunForJobRow
		should bool
	}
	value, err := withTx(ctx, r.pool, func(queries *db.Queries) (result, error) {
		run, err := queries.GetTranscriptionRunForJob(ctx, runID)
		if err != nil {
			return result{}, err
		}
		if run.Status == "queued" {
			if _, err := queries.SetRunDownloadStarted(ctx, runID); err != nil {
				return result{}, err
			}
			if err := transcriptionEvent(ctx, queries, runID, "audio_download_started", nil); err != nil {
				return result{}, err
			}
			run.Status, run.Stage = "downloading", "downloading_audio"
		}
		return result{run: run, should: run.Status == "downloading" && run.SourceObjectKey == nil}, nil
	})
	return value.run, value.should, err
}

func (r *TranscriptionRepository) FinishDownload(ctx context.Context, runID pgtype.UUID, key, hash string) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.SetRunDownloaded(ctx, db.SetRunDownloadedParams{SourceObjectKey: &key, SourceAudioHash: &hash, RunID: runID})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, PrepareAudioJobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, runID, "audio_downloaded", nil); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return wrap("finish audio download", err)
}

func (r *TranscriptionRepository) GetRunForJob(ctx context.Context, runID pgtype.UUID) (db.GetTranscriptionRunForJobRow, error) {
	return db.New(r.pool).GetTranscriptionRunForJob(ctx, runID)
}

func (r *TranscriptionRepository) FinishPrepare(ctx context.Context, runID pgtype.UUID, key, hash string, durationMS int64) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.SetRunPrepared(ctx, db.SetRunPreparedParams{
			PreparedObjectKey: &key, PreparedAudioHash: &hash, DurationMs: &durationMS, RunID: runID,
		})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, PlanTranscriptionJobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, runID, "audio_prepared", map[string]any{"duration_ms": durationMS}); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return wrap("finish audio preparation", err)
}

func (r *TranscriptionRepository) Plan(ctx context.Context, runID pgtype.UUID, windows []domain.Window) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.LockTranscriptionRun(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if run.TotalChunks > 0 {
			return run, nil
		}
		if run.Status != "preparing" || run.PreparedObjectKey == nil || run.DurationMs == nil {
			return db.TranscriptionRun{}, errors.New("run is not ready for chunk planning")
		}
		chunks := make([]db.TranscriptionChunk, 0, len(windows))
		for _, window := range windows {
			chunk, err := queries.CreateTranscriptionChunk(ctx, db.CreateTranscriptionChunkParams{
				RunID: runID, Sequence: int32(window.Sequence), CoreStartMs: window.CoreStartMS, CoreEndMs: window.CoreEndMS,
				RenderStartMs: window.RenderStartMS, RenderEndMs: window.RenderEndMS,
			})
			if err != nil {
				return db.TranscriptionRun{}, err
			}
			chunks = append(chunks, chunk)
		}
		run, err = queries.SetRunTranscribing(ctx, db.SetRunTranscribingParams{TotalChunks: int32(len(chunks)), RunID: runID})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		for _, chunk := range chunks {
			if _, err := enqueue(ctx, queries, chunkJob(run, chunk, RenderAudioChunkJobType, time.Time{})); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: "running", EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, runID, "chunks_planned", map[string]any{"total_chunks": len(chunks)}); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	return wrap("plan transcription chunks", err)
}

func (r *TranscriptionRepository) BeginRender(ctx context.Context, chunkID pgtype.UUID) (db.GetTranscriptionChunkForJobRow, bool, error) {
	chunk, err := db.New(r.pool).GetTranscriptionChunkForJob(ctx, chunkID)
	if err != nil {
		return chunk, false, err
	}
	if chunk.RunStatus != "transcribing" || (chunk.Status != "planned" && chunk.Status != "rendering") {
		return chunk, false, nil
	}
	if chunk.Status == "planned" {
		if _, err := db.New(r.pool).StartChunkRender(ctx, chunkID); err != nil {
			return chunk, false, err
		}
		chunk.Status = "rendering"
	}
	return chunk, true, nil
}

func (r *TranscriptionRepository) FinishRender(ctx context.Context, chunkID pgtype.UUID, key, hash, fingerprint string) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionChunk, error) {
		chunk, err := queries.SetChunkRendered(ctx, db.SetChunkRenderedParams{
			ObjectKey: &key, AudioHash: &hash, Fingerprint: &fingerprint, ChunkID: chunkID,
		})
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		run, err := queries.LockTranscriptionRun(ctx, chunk.TranscriptionRunID)
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		if _, err := enqueue(ctx, queries, chunkJob(run, chunk, SubmitASRJobType, time.Time{})); err != nil {
			return db.TranscriptionChunk{}, err
		}
		return chunk, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return wrap("finish chunk render", err)
}

func (r *TranscriptionRepository) BeginSubmit(ctx context.Context, chunkID pgtype.UUID) (db.GetTranscriptionChunkForJobRow, bool, error) {
	chunk, err := db.New(r.pool).GetTranscriptionChunkForJob(ctx, chunkID)
	if err != nil {
		return chunk, false, err
	}
	if chunk.RunStatus != "transcribing" {
		return chunk, false, nil
	}
	switch chunk.Status {
	case "ready":
		if _, err := db.New(r.pool).StartChunkSubmit(ctx, chunkID); err != nil {
			return chunk, false, err
		}
		chunk.Status = "submitting"
		return chunk, true, nil
	case "submitting":
		if chunk.ExternalTaskID == nil {
			return chunk, false, ErrSubmissionAmbiguous
		}
		return chunk, false, nil
	default:
		return chunk, false, nil
	}
}

func (r *TranscriptionRepository) ResetSubmit(ctx context.Context, chunkID pgtype.UUID) error {
	return db.New(r.pool).ResetChunkSubmit(ctx, chunkID)
}

func (r *TranscriptionRepository) FinishSubmit(ctx context.Context, chunkID pgtype.UUID, taskID string, pollDelay time.Duration) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionChunk, error) {
		chunk, err := queries.SetChunkSubmitted(ctx, db.SetChunkSubmittedParams{ExternalTaskID: &taskID, ChunkID: chunkID})
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		run, err := queries.LockTranscriptionRun(ctx, chunk.TranscriptionRunID)
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		if _, err := enqueue(ctx, queries, chunkJob(run, chunk, PollASRJobType, time.Now().Add(pollDelay))); err != nil {
			return db.TranscriptionChunk{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "chunk_transcription_started", map[string]any{"chunk": chunk.Sequence + 1, "total": run.TotalChunks}); err != nil {
			return db.TranscriptionChunk{}, err
		}
		return chunk, nil
	})
	return wrap("finish ASR submit", err)
}

func (r *TranscriptionRepository) GetChunkForJob(ctx context.Context, chunkID pgtype.UUID) (db.GetTranscriptionChunkForJobRow, error) {
	return db.New(r.pool).GetTranscriptionChunkForJob(ctx, chunkID)
}

func (r *TranscriptionRepository) MarkPolling(ctx context.Context, chunkID pgtype.UUID) error {
	return db.New(r.pool).SetChunkPolling(ctx, chunkID)
}

func (r *TranscriptionRepository) ClearExternalTask(ctx context.Context, chunkID pgtype.UUID) error {
	return db.New(r.pool).ClearChunkExternalTask(ctx, chunkID)
}

func (r *TranscriptionRepository) FinishPoll(ctx context.Context, chunkID pgtype.UUID, resultURL string) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionChunk, error) {
		chunk, err := queries.SetChunkResultReady(ctx, db.SetChunkResultReadyParams{ResultUrl: &resultURL, ChunkID: chunkID})
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		run, err := queries.LockTranscriptionRun(ctx, chunk.TranscriptionRunID)
		if err != nil {
			return db.TranscriptionChunk{}, err
		}
		if _, err := enqueue(ctx, queries, chunkJob(run, chunk, IngestASRResultJobType, time.Time{})); err != nil {
			return db.TranscriptionChunk{}, err
		}
		return chunk, nil
	})
	return wrap("finish ASR poll", err)
}

func (r *TranscriptionRepository) FinishIngest(ctx context.Context, chunkID pgtype.UUID, rawKey string, result domain.Result) error {
	normalized, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		chunk, err := queries.SetChunkIngested(ctx, db.SetChunkIngestedParams{RawResultObjectKey: &rawKey, NormalizedResult: normalized, ChunkID: chunkID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.TranscriptionRun{}, nil
		}
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		run, err := queries.IncrementRunCompletedChunks(ctx, chunk.TranscriptionRunID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "chunk_transcription_completed", map[string]any{
			"chunk": chunk.Sequence + 1, "completed_chunks": run.CompletedChunks, "total_chunks": run.TotalChunks,
		}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if run.CompletedChunks == run.TotalChunks {
			if _, err := enqueue(ctx, queries, runJob(run, AlignSpeakersJobType, time.Time{}, nil)); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		return run, nil
	})
	return wrap("finish ASR ingest", err)
}

func (r *TranscriptionRepository) BeginAlignment(ctx context.Context, runID pgtype.UUID) (db.TranscriptionRun, []db.TranscriptionChunk, bool, error) {
	var chunks []db.TranscriptionChunk
	run, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.LockTranscriptionRun(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if run.Status != "transcribing" && run.Status != "aligning" {
			return run, nil
		}
		first := run.Status == "transcribing"
		run, err = queries.SetRunAligning(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		chunks, err = queries.ListRunChunks(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if first {
			if err := transcriptionEvent(ctx, queries, runID, "speaker_alignment_started", nil); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		return run, nil
	})
	if err != nil {
		return db.TranscriptionRun{}, nil, false, err
	}
	return run, chunks, run.Status == "aligning", nil
}

func (r *TranscriptionRepository) FinishAlignment(ctx context.Context, run db.TranscriptionRun, chunks []db.TranscriptionChunk, alignment domain.Alignment) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		locked, err := queries.LockTranscriptionRun(ctx, run.ID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if locked.Status == "merging" || locked.Status == "completed" {
			return locked, nil
		}
		if locked.Status != "aligning" || len(chunks) != len(alignment.SpeakerMaps) {
			return db.TranscriptionRun{}, errors.New("run is not ready to save alignment")
		}
		low := make(map[int]bool, len(alignment.LowConfidenceChunk))
		for _, sequence := range alignment.LowConfidenceChunk {
			low[sequence] = true
		}
		for index, chunk := range chunks {
			encoded, err := json.Marshal(alignment.SpeakerMaps[index])
			if err != nil {
				return db.TranscriptionRun{}, err
			}
			if err := queries.SetChunkSpeakerMap(ctx, db.SetChunkSpeakerMapParams{SpeakerMap: encoded, LowConfidence: low[int(chunk.Sequence)], ChunkID: chunk.ID}); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		locked, err = queries.SetRunMerging(ctx, run.ID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(locked, MergeTranscriptJobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if len(alignment.LowConfidenceChunk) > 0 {
			if err := transcriptionEvent(ctx, queries, run.ID, "speaker_alignment_low_confidence", map[string]any{"chunks": alignment.LowConfidenceChunk}); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		return locked, nil
	})
	return wrap("finish speaker alignment", err)
}

func (r *TranscriptionRepository) Chunks(ctx context.Context, runID pgtype.UUID) ([]db.TranscriptionChunk, error) {
	return db.New(r.pool).ListRunChunks(ctx, runID)
}

func (r *TranscriptionRepository) ActivateTranscript(
	ctx context.Context,
	runID pgtype.UUID,
	speakers []domain.Speaker,
	segments []domain.MergedSegment,
) (db.TranscriptVersion, error) {
	version, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptVersion, error) {
		run, err := queries.LockTranscriptionRun(ctx, runID)
		if err != nil {
			return db.TranscriptVersion{}, err
		}
		if run.Status != "merging" {
			if run.Status == "completed" {
				return queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: run.EpisodeID, UserID: run.UserID})
			}
			return db.TranscriptVersion{}, errors.New("run is not ready to merge")
		}
		if _, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptVersion{}, err
		}
		next, err := queries.NextTranscriptVersion(ctx, run.EpisodeID)
		if err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := queries.DeactivateTranscriptVersions(ctx, run.EpisodeID); err != nil {
			return db.TranscriptVersion{}, err
		}
		version, err := queries.CreateTranscriptVersion(ctx, db.CreateTranscriptVersionParams{
			UserID: run.UserID, EpisodeID: run.EpisodeID, RunID: run.ID, Version: next,
		})
		if err != nil {
			return db.TranscriptVersion{}, err
		}
		speakerIDs := make(map[string]pgtype.UUID, len(speakers))
		for _, input := range speakers {
			speaker, err := queries.CreateTranscriptSpeaker(ctx, db.CreateTranscriptSpeakerParams{
				TranscriptID: version.ID, StableKey: input.StableKey, DisplayName: input.DisplayName, Role: "unknown",
			})
			if err != nil {
				return db.TranscriptVersion{}, err
			}
			speakerIDs[input.StableKey] = speaker.ID
		}
		for _, input := range segments {
			chunkID, err := parseID(input.SourceChunkID)
			if err != nil {
				return db.TranscriptVersion{}, err
			}
			words, err := json.Marshal(input.Words)
			if err != nil {
				return db.TranscriptVersion{}, err
			}
			if _, err := queries.CreateTranscriptSegment(ctx, db.CreateTranscriptSegmentParams{
				TranscriptID: version.ID, SpeakerID: speakerIDs[input.SpeakerKey], Sequence: int32(input.Sequence),
				StartMs: input.StartMS, EndMs: input.EndMS, Text: input.Text, Words: words, SourceChunkID: chunkID,
			}); err != nil {
				return db.TranscriptVersion{}, err
			}
		}
		run, err = queries.CompleteTranscriptionRun(ctx, run.ID)
		if err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: "completed", EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "transcript_merged", map[string]any{"transcript_id": idText(version.ID), "version": version.Version}); err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "completed", nil); err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := markEpisodeAIStale(ctx, queries, run.UserID, run.EpisodeID); err != nil {
			return db.TranscriptVersion{}, err
		}
		if err := enqueueSearchBuild(ctx, queries, run.UserID, run.EpisodeID); err != nil {
			return db.TranscriptVersion{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, CleanupAudioJobType, time.Time{}, map[string]string{"scope": "audio"})); err != nil {
			return db.TranscriptVersion{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, CleanupAudioJobType, time.Now().Add(chunkRetention), map[string]string{"scope": "chunks"})); err != nil {
			return db.TranscriptVersion{}, err
		}
		return version, nil
	})
	if err != nil {
		return db.TranscriptVersion{}, fmt.Errorf("activate transcript: %w", err)
	}
	return version, nil
}

func (r *TranscriptionRepository) FailJob(ctx context.Context, job db.Job, failure error) error {
	code, message := classifyFailure(failure)
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		runID := job.EntityID
		if job.EntityType == TranscriptionChunkEntity {
			chunk, err := queries.GetTranscriptionChunkForJob(ctx, job.EntityID)
			if errors.Is(err, pgx.ErrNoRows) {
				return db.TranscriptionRun{}, nil
			}
			if err != nil {
				return db.TranscriptionRun{}, err
			}
			runID = chunk.TranscriptionRunID
			if err := queries.FailTranscriptionChunk(ctx, db.FailTranscriptionChunkParams{ErrorCode: &code, ErrorMessage: &message, ChunkID: job.EntityID}); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		run, err := queries.FailTranscriptionRun(ctx, db.FailTranscriptionRunParams{ErrorCode: &code, ErrorMessage: &message, RunID: runID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.TranscriptionRun{}, nil
		}
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: "failed", EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "failed", map[string]string{"code": code, "message": message}); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	return wrap("fail transcription run", err)
}

func (r *TranscriptionRepository) Retry(ctx context.Context, userID, runID pgtype.UUID) (db.TranscriptionRun, error) {
	run, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.LockTranscriptionRun(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if run.UserID != userID {
			return db.TranscriptionRun{}, pgx.ErrNoRows
		}
		if run.Status != "failed" {
			return db.TranscriptionRun{}, ErrTranscriptionNotRetryable
		}
		chunks, err := queries.ListRunChunks(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		status, stage, jobType := "queued", "retrying_download", DownloadAudioJobType
		var retryChunks []db.TranscriptionChunk
		switch {
		case run.SourceObjectKey == nil:
		case run.PreparedObjectKey == nil:
			status, stage, jobType = "preparing", "retrying_prepare", PrepareAudioJobType
		case len(chunks) == 0:
			status, stage, jobType = "preparing", "retrying_plan", PlanTranscriptionJobType
		default:
			for _, chunk := range chunks {
				if chunk.Status == "failed" {
					reset, err := queries.ResetFailedChunk(ctx, chunk.ID)
					if err != nil {
						return db.TranscriptionRun{}, err
					}
					retryChunks = append(retryChunks, reset)
				}
			}
			status, stage = "transcribing", "retrying_chunks"
			if len(retryChunks) == 0 {
				status, stage, jobType = "aligning", "retrying_alignment", AlignSpeakersJobType
				allMapped := true
				for _, chunk := range chunks {
					allMapped = allMapped && len(chunk.SpeakerMap) > 0
				}
				if allMapped {
					status, stage, jobType = "merging", "retrying_merge", MergeTranscriptJobType
				}
			}
		}
		run, err = queries.ResetTranscriptionRun(ctx, db.ResetTranscriptionRunParams{Status: status, Stage: stage, RunID: runID})
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if len(retryChunks) > 0 {
			for _, chunk := range retryChunks {
				kind := SubmitASRJobType
				switch chunk.Status {
				case "planned":
					kind = RenderAudioChunkJobType
				case "submitted":
					kind = PollASRJobType
				case "running":
					kind = IngestASRResultJobType
				}
				if _, err := enqueue(ctx, queries, chunkJob(run, chunk, kind, time.Time{})); err != nil {
					return db.TranscriptionRun{}, err
				}
			}
		} else if _, err := enqueue(ctx, queries, runJob(run, jobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: "queued", EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "retried", nil); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	if activeTranscriptionConflict(err) {
		return db.TranscriptionRun{}, ErrTranscriptionRunning
	}
	if err != nil {
		return db.TranscriptionRun{}, fmt.Errorf("retry transcription: %w", err)
	}
	return run, nil
}

func (r *TranscriptionRepository) Cancel(ctx context.Context, userID, runID pgtype.UUID) (db.TranscriptionRun, error) {
	run, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptionRun, error) {
		run, err := queries.CancelTranscriptionRun(ctx, db.CancelTranscriptionRunParams{RunID: runID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return queries.GetTranscriptionRun(ctx, db.GetTranscriptionRunParams{RunID: runID, UserID: userID})
		}
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := queries.CancelRunChunks(ctx, runID); err != nil {
			return db.TranscriptionRun{}, err
		}
		jobs, err := queries.CancelRunJobs(ctx, runID)
		if err != nil {
			return db.TranscriptionRun{}, err
		}
		for _, job := range jobs {
			if err := createEvent(ctx, queries, job, "canceled"); err != nil {
				return db.TranscriptionRun{}, err
			}
		}
		episodeStatus := "waiting"
		if _, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: run.EpisodeID, UserID: run.UserID}); err == nil {
			episodeStatus = "completed"
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.TranscriptionRun{}, err
		}
		if err := queries.SetEpisodeTranscriptionStatus(ctx, db.SetEpisodeTranscriptionStatusParams{Status: episodeStatus, EpisodeID: run.EpisodeID, UserID: run.UserID}); err != nil {
			return db.TranscriptionRun{}, err
		}
		if _, err := enqueue(ctx, queries, runJob(run, CancelASRJobType, time.Time{}, nil)); err != nil {
			return db.TranscriptionRun{}, err
		}
		if err := transcriptionEvent(ctx, queries, run.ID, "canceled", nil); err != nil {
			return db.TranscriptionRun{}, err
		}
		return run, nil
	})
	if err != nil {
		return db.TranscriptionRun{}, fmt.Errorf("cancel transcription: %w", err)
	}
	return run, nil
}

func (r *TranscriptionRepository) ExternalTaskIDs(ctx context.Context, runID pgtype.UUID) ([]string, error) {
	values, err := db.New(r.pool).ListRunExternalTaskIDs(ctx, runID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, *value)
		}
	}
	return result, nil
}

type CleanupObjects struct {
	AudioKeys []string
	ChunkKeys []string
}

func (r *TranscriptionRepository) CleanupObjects(ctx context.Context, runID pgtype.UUID) (CleanupObjects, error) {
	run, err := db.New(r.pool).GetTranscriptionRunForJob(ctx, runID)
	if err != nil {
		return CleanupObjects{}, err
	}
	objects := CleanupObjects{}
	for _, value := range []*string{run.SourceObjectKey, run.PreparedObjectKey} {
		if value != nil {
			objects.AudioKeys = append(objects.AudioKeys, *value)
		}
	}
	keys, err := db.New(r.pool).ListRunChunkObjectKeys(ctx, runID)
	if err != nil {
		return CleanupObjects{}, err
	}
	for _, key := range keys {
		if key != nil {
			objects.ChunkKeys = append(objects.ChunkKeys, *key)
		}
	}
	return objects, nil
}

func (r *TranscriptionRepository) MarkCleaned(ctx context.Context, runID pgtype.UUID, scope string) error {
	switch scope {
	case "audio":
		return db.New(r.pool).MarkRunAudioCleaned(ctx, runID)
	case "chunks":
		return db.New(r.pool).MarkRunChunksCleaned(ctx, runID)
	default:
		return errors.New("unknown cleanup scope")
	}
}

type ActiveTranscript struct {
	Version  db.TranscriptVersion
	Speakers []db.TranscriptSpeaker
}

func (r *TranscriptionRepository) ActiveTranscript(ctx context.Context, userID, episodeID pgtype.UUID) (ActiveTranscript, error) {
	queries := db.New(r.pool)
	version, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return ActiveTranscript{}, err
	}
	speakers, err := queries.ListTranscriptSpeakers(ctx, db.ListTranscriptSpeakersParams{TranscriptID: version.ID, UserID: userID})
	if err != nil {
		return ActiveTranscript{}, err
	}
	return ActiveTranscript{Version: version, Speakers: speakers}, nil
}

func (r *TranscriptionRepository) Segments(ctx context.Context, userID, transcriptID pgtype.UUID, limit, offset int32) ([]db.TranscriptSegment, int64, error) {
	queries := db.New(r.pool)
	if _, err := queries.GetTranscriptVersion(ctx, db.GetTranscriptVersionParams{TranscriptID: transcriptID, UserID: userID}); err != nil {
		return nil, 0, err
	}
	segments, err := queries.ListTranscriptSegments(ctx, db.ListTranscriptSegmentsParams{TranscriptID: transcriptID, UserID: userID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountTranscriptSegments(ctx, db.CountTranscriptSegmentsParams{TranscriptID: transcriptID, UserID: userID})
	return segments, total, err
}

func (r *TranscriptionRepository) RenameSpeaker(ctx context.Context, userID, transcriptID, speakerID pgtype.UUID, displayName, role string) (db.TranscriptSpeaker, error) {
	displayName, role = strings.TrimSpace(displayName), strings.TrimSpace(role)
	if displayName == "" {
		return db.TranscriptSpeaker{}, errors.New("display name is required")
	}
	speaker, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptSpeaker, error) {
		version, err := queries.GetTranscriptVersion(ctx, db.GetTranscriptVersionParams{TranscriptID: transcriptID, UserID: userID})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if _, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: version.EpisodeID, UserID: userID}); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		current, err := queries.LockTranscriptSpeaker(ctx, db.LockTranscriptSpeakerParams{SpeakerID: speakerID, TranscriptID: transcriptID, UserID: userID})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if role == "" {
			role = current.Role
		}
		updated, err := queries.RenameTranscriptSpeaker(ctx, db.RenameTranscriptSpeakerParams{
			DisplayName: displayName, Role: role, SpeakerID: speakerID, TranscriptID: transcriptID, UserID: userID,
		})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if err := markEpisodeAIStale(ctx, queries, userID, version.EpisodeID); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if err := enqueueSearchBuild(ctx, queries, userID, version.EpisodeID); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		return updated, nil
	})
	return speaker, wrap("rename transcript speaker", err)
}

func (r *TranscriptionRepository) MergeSpeakers(ctx context.Context, userID, transcriptID, sourceID, targetID pgtype.UUID) (db.TranscriptSpeaker, error) {
	if sourceID == targetID {
		return db.TranscriptSpeaker{}, errors.New("source and target speakers must differ")
	}
	target, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.TranscriptSpeaker, error) {
		version, err := queries.GetTranscriptVersion(ctx, db.GetTranscriptVersionParams{TranscriptID: transcriptID, UserID: userID})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if _, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: version.EpisodeID, UserID: userID}); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		firstID, secondID := sourceID, targetID
		if firstID.String() > secondID.String() {
			firstID, secondID = secondID, firstID
		}
		first, err := queries.LockTranscriptSpeaker(ctx, db.LockTranscriptSpeakerParams{SpeakerID: firstID, TranscriptID: transcriptID, UserID: userID})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		second, err := queries.LockTranscriptSpeaker(ctx, db.LockTranscriptSpeakerParams{SpeakerID: secondID, TranscriptID: transcriptID, UserID: userID})
		if err != nil {
			return db.TranscriptSpeaker{}, err
		}
		target := second
		if first.ID == targetID {
			target = first
		}
		if err := queries.MoveTranscriptSegments(ctx, db.MoveTranscriptSegmentsParams{TargetSpeakerID: targetID, TranscriptID: transcriptID, SourceSpeakerID: sourceID}); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if err := queries.DeleteTranscriptSpeaker(ctx, db.DeleteTranscriptSpeakerParams{SpeakerID: sourceID, TranscriptID: transcriptID}); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if err := markEpisodeAIStale(ctx, queries, userID, version.EpisodeID); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		if err := enqueueSearchBuild(ctx, queries, userID, version.EpisodeID); err != nil {
			return db.TranscriptSpeaker{}, err
		}
		return target, nil
	})
	return target, wrap("merge transcript speakers", err)
}

func activeTranscriptionConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "transcription_runs_episode_active_idx"
}

func runJob(run db.TranscriptionRun, jobType string, runAfter time.Time, payload any) NewJob {
	encoded := json.RawMessage(`{}`)
	if payload != nil {
		encoded, _ = json.Marshal(payload)
	}
	return NewJob{
		UserID: run.UserID, Type: jobType, EntityType: TranscriptionRunEntity, EntityID: run.ID,
		Payload: encoded, MaxAttempts: 3, RunAfter: runAfter,
	}
}

func chunkJob(run db.TranscriptionRun, chunk db.TranscriptionChunk, jobType string, runAfter time.Time) NewJob {
	return NewJob{
		UserID: run.UserID, Type: jobType, EntityType: TranscriptionChunkEntity, EntityID: chunk.ID,
		MaxAttempts: 3, RunAfter: runAfter,
	}
}

func transcriptionEvent(ctx context.Context, queries *db.Queries, runID pgtype.UUID, eventType string, data any) error {
	encoded := []byte(`{}`)
	if data != nil {
		var err error
		encoded, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}
	_, err := queries.CreateTranscriptionEvent(ctx, db.CreateTranscriptionEventParams{RunID: runID, EventType: eventType, Data: encoded})
	return err
}

func classifyFailure(err error) (string, string) {
	code, message := "TRANSCRIPTION_JOB_FAILED", "transcription job failed"
	var classified interface{ Code() string }
	if errors.As(err, &classified) {
		code = classified.Code()
	}
	if err != nil {
		message = err.Error()
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	return code, message
}

func parseID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func idText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
