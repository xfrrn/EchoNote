-- name: LockEpisodeForTranscription :one
SELECT
    episode.id,
    episode.resolve_status,
    episode.transcription_status,
    COALESCE((
        SELECT source.audio_url
        FROM episode_sources AS source
        WHERE source.episode_id = episode.id
          AND source.user_id = episode.user_id
        ORDER BY source.created_at DESC, source.id DESC
        LIMIT 1
    ), '')::text AS audio_url
FROM episodes AS episode
WHERE episode.id = sqlc.arg(episode_id)
  AND episode.user_id = sqlc.arg(user_id)
FOR UPDATE OF episode;

-- name: CreateTranscriptionRun :one
INSERT INTO transcription_runs (
    user_id,
    episode_id,
    profile,
    provider,
    model,
    config
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(profile),
    sqlc.arg(provider),
    sqlc.arg(model),
    sqlc.arg(config)::jsonb
)
RETURNING *;

-- name: GetLatestTranscriptionRunForEpisode :one
SELECT *
FROM transcription_runs
WHERE episode_id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetTranscriptionRun :one
SELECT *
FROM transcription_runs
WHERE id = sqlc.arg(run_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetTranscriptionRunForJob :one
SELECT
    run.*,
    COALESCE((
        SELECT source.audio_url
        FROM episode_sources AS source
        WHERE source.episode_id = run.episode_id
          AND source.user_id = run.user_id
        ORDER BY source.created_at DESC, source.id DESC
        LIMIT 1
    ), '')::text AS audio_url
FROM transcription_runs AS run
WHERE run.id = sqlc.arg(run_id);

-- name: LockTranscriptionRun :one
SELECT *
FROM transcription_runs
WHERE id = sqlc.arg(run_id)
FOR UPDATE;

-- name: SetEpisodeTranscriptionStatus :exec
UPDATE episodes
SET transcription_status = sqlc.arg(status),
    updated_at = now()
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id);

-- name: CreateTranscriptionEvent :one
INSERT INTO transcription_events (transcription_run_id, event_type, data)
VALUES (sqlc.arg(run_id), sqlc.arg(event_type), sqlc.arg(data)::jsonb)
RETURNING *;

-- name: SetRunDownloadStarted :one
UPDATE transcription_runs
SET status = 'downloading',
    stage = 'downloading_audio',
    started_at = COALESCE(started_at, now()),
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'queued'
RETURNING *;

-- name: SetRunDownloaded :one
UPDATE transcription_runs
SET status = 'preparing',
    stage = 'preparing_audio',
    source_object_key = sqlc.arg(source_object_key),
    source_audio_hash = sqlc.arg(source_audio_hash),
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'downloading'
RETURNING *;

-- name: SetRunPrepared :one
UPDATE transcription_runs
SET stage = 'planning_chunks',
    prepared_object_key = sqlc.arg(prepared_object_key),
    prepared_audio_hash = sqlc.arg(prepared_audio_hash),
    duration_ms = sqlc.arg(duration_ms),
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'preparing'
RETURNING *;

-- name: CreateTranscriptionChunk :one
INSERT INTO transcription_chunks (
    transcription_run_id,
    sequence,
    core_start_ms,
    core_end_ms,
    render_start_ms,
    render_end_ms
) VALUES (
    sqlc.arg(run_id),
    sqlc.arg(sequence),
    sqlc.arg(core_start_ms),
    sqlc.arg(core_end_ms),
    sqlc.arg(render_start_ms),
    sqlc.arg(render_end_ms)
)
RETURNING *;

-- name: SetRunTranscribing :one
UPDATE transcription_runs
SET status = 'transcribing',
    stage = 'rendering_chunks',
    total_chunks = sqlc.arg(total_chunks),
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'preparing'
  AND total_chunks = 0
RETURNING *;

-- name: GetTranscriptionChunkForJob :one
SELECT
    chunk.*,
    run.user_id,
    run.episode_id,
    run.provider,
    run.model,
    run.config,
    run.status AS run_status,
    run.prepared_object_key,
    run.duration_ms
FROM transcription_chunks AS chunk
JOIN transcription_runs AS run ON run.id = chunk.transcription_run_id
WHERE chunk.id = sqlc.arg(chunk_id);

-- name: StartChunkRender :one
UPDATE transcription_chunks
SET status = 'rendering',
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'planned'
RETURNING *;

-- name: SetChunkRendered :one
UPDATE transcription_chunks
SET status = 'ready',
    object_key = sqlc.arg(object_key),
    audio_hash = sqlc.arg(audio_hash),
    fingerprint = sqlc.arg(fingerprint),
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'rendering'
RETURNING *;

-- name: StartChunkSubmit :one
UPDATE transcription_chunks
SET status = 'submitting',
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'ready'
RETURNING *;

-- name: ResetChunkSubmit :exec
UPDATE transcription_chunks
SET status = 'ready',
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'submitting'
  AND external_task_id IS NULL;

-- name: SetChunkSubmitted :one
UPDATE transcription_chunks
SET status = 'submitted',
    external_task_id = sqlc.arg(external_task_id),
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'submitting'
  AND external_task_id IS NULL
RETURNING *;

-- name: SetChunkPolling :exec
UPDATE transcription_chunks
SET status = 'running',
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status IN ('submitted', 'running');

-- name: SetChunkResultReady :one
UPDATE transcription_chunks
SET status = 'running',
    result_url = sqlc.arg(result_url),
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status IN ('submitted', 'running')
RETURNING *;

-- name: ClearChunkExternalTask :exec
UPDATE transcription_chunks
SET external_task_id = NULL,
    result_url = NULL,
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status IN ('submitted', 'running');

-- name: SetChunkIngested :one
UPDATE transcription_chunks
SET status = 'completed',
    raw_result_object_key = sqlc.arg(raw_result_object_key),
    normalized_result = sqlc.arg(normalized_result)::jsonb,
    error_code = NULL,
    error_message = NULL,
    updated_at = now(),
    completed_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'running'
  AND result_url IS NOT NULL
RETURNING *;

-- name: IncrementRunCompletedChunks :one
UPDATE transcription_runs
SET completed_chunks = completed_chunks + 1,
    stage = CASE WHEN completed_chunks + 1 = total_chunks THEN 'speaker_alignment_queued' ELSE 'transcribing_chunks' END,
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'transcribing'
  AND completed_chunks < total_chunks
RETURNING *;

-- name: ListRunChunks :many
SELECT *
FROM transcription_chunks
WHERE transcription_run_id = sqlc.arg(run_id)
ORDER BY sequence;

-- name: SetRunAligning :one
UPDATE transcription_runs
SET status = 'aligning',
    stage = 'aligning_speakers',
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status IN ('transcribing', 'aligning')
  AND completed_chunks = total_chunks
  AND total_chunks > 0
RETURNING *;

-- name: SetChunkSpeakerMap :exec
UPDATE transcription_chunks
SET speaker_map = sqlc.arg(speaker_map)::jsonb,
    alignment_low_confidence = sqlc.arg(low_confidence),
    updated_at = now()
WHERE id = sqlc.arg(chunk_id)
  AND status = 'completed';

-- name: SetRunMerging :one
UPDATE transcription_runs
SET status = 'merging',
    stage = 'merging_transcript',
    updated_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'aligning'
RETURNING *;

-- name: NextTranscriptVersion :one
SELECT COALESCE(max(version), 0)::int + 1
FROM transcript_versions
WHERE episode_id = sqlc.arg(episode_id);

-- name: DeactivateTranscriptVersions :exec
UPDATE transcript_versions
SET is_active = false
WHERE episode_id = sqlc.arg(episode_id)
  AND is_active;

-- name: CreateTranscriptVersion :one
INSERT INTO transcript_versions (
    user_id,
    episode_id,
    transcription_run_id,
    version,
    is_active
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(run_id),
    sqlc.arg(version),
    true
)
RETURNING *;

-- name: CreateTranscriptSpeaker :one
INSERT INTO transcript_speakers (
    transcript_version_id,
    stable_key,
    display_name,
    role
) VALUES (
    sqlc.arg(transcript_id),
    sqlc.arg(stable_key),
    sqlc.arg(display_name),
    sqlc.arg(role)
)
RETURNING *;

-- name: CreateTranscriptSegment :one
INSERT INTO transcript_segments (
    transcript_version_id,
    speaker_id,
    sequence,
    start_ms,
    end_ms,
    text,
    words,
    source_chunk_id
) VALUES (
    sqlc.arg(transcript_id),
    sqlc.arg(speaker_id),
    sqlc.arg(sequence),
    sqlc.arg(start_ms),
    sqlc.arg(end_ms),
    sqlc.arg(text),
    sqlc.arg(words)::jsonb,
    sqlc.arg(source_chunk_id)
)
RETURNING *;

-- name: CompleteTranscriptionRun :one
UPDATE transcription_runs
SET status = 'completed',
    stage = 'completed',
    error_code = NULL,
    error_message = NULL,
    updated_at = now(),
    completed_at = now()
WHERE id = sqlc.arg(run_id)
  AND status = 'merging'
RETURNING *;

-- name: FailTranscriptionRun :one
UPDATE transcription_runs
SET status = 'failed',
    stage = 'failed',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    updated_at = now(),
    completed_at = now()
WHERE id = sqlc.arg(run_id)
  AND status NOT IN ('completed', 'failed', 'canceled')
RETURNING *;

-- name: FailTranscriptionChunk :exec
UPDATE transcription_chunks
SET status = 'failed',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    updated_at = now(),
    completed_at = NULL
WHERE id = sqlc.arg(chunk_id)
  AND status NOT IN ('completed', 'failed', 'canceled');

-- name: ListRunChunkObjectKeys :many
SELECT object_key
FROM transcription_chunks
WHERE transcription_run_id = sqlc.arg(run_id)
  AND object_key IS NOT NULL
ORDER BY sequence;

-- name: MarkRunAudioCleaned :exec
UPDATE transcription_runs
SET audio_cleaned_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(run_id);

-- name: MarkRunChunksCleaned :exec
UPDATE transcription_runs
SET chunks_cleaned_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(run_id);

-- name: GetActiveTranscriptVersion :one
SELECT version.*
FROM transcript_versions AS version
WHERE version.episode_id = sqlc.arg(episode_id)
  AND version.user_id = sqlc.arg(user_id)
  AND version.is_active;

-- name: GetTranscriptVersion :one
SELECT *
FROM transcript_versions
WHERE id = sqlc.arg(transcript_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListTranscriptSpeakers :many
SELECT speaker.*
FROM transcript_speakers AS speaker
JOIN transcript_versions AS version ON version.id = speaker.transcript_version_id
WHERE speaker.transcript_version_id = sqlc.arg(transcript_id)
  AND version.user_id = sqlc.arg(user_id)
ORDER BY speaker.stable_key;

-- name: ListTranscriptSegments :many
SELECT segment.*
FROM transcript_segments AS segment
JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
WHERE segment.transcript_version_id = sqlc.arg(transcript_id)
  AND version.user_id = sqlc.arg(user_id)
ORDER BY segment.sequence
LIMIT sqlc.arg(page_limit)::int
OFFSET sqlc.arg(page_offset)::int;

-- name: CountTranscriptSegments :one
SELECT count(*)
FROM transcript_segments AS segment
JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
WHERE segment.transcript_version_id = sqlc.arg(transcript_id)
  AND version.user_id = sqlc.arg(user_id);
