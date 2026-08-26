-- name: CreateImport :one
INSERT INTO imports (user_id, submitted_url)
VALUES (sqlc.arg(user_id), sqlc.arg(submitted_url))
RETURNING *;

-- name: SetImportJob :exec
UPDATE imports
SET job_id = sqlc.arg(job_id),
    updated_at = now()
WHERE id = sqlc.arg(import_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetImportStatus :one
SELECT
    import_record.id,
    import_record.submitted_url,
    import_record.episode_id,
    episode.title,
    episode.duration_ms,
    COALESCE(job.status, CASE WHEN episode.resolve_status = 'completed' THEN 'succeeded' ELSE 'failed' END)::text AS import_status,
    COALESCE(job.stage, CASE WHEN episode.resolve_status = 'completed' THEN 'completed' ELSE 'expired' END)::text AS import_stage,
    job.error_code,
    job.error_message,
    run.id AS transcription_run_id,
    COALESCE(run.status, '')::text AS transcription_status,
    COALESCE(run.stage, '')::text AS transcription_stage,
    COALESCE(run.total_chunks, 0)::integer AS total_chunks,
    COALESCE(run.completed_chunks, 0)::integer AS completed_chunks,
    run.error_code AS transcription_error_code,
    run.error_message AS transcription_error_message,
    transcript.id AS transcript_id,
    import_record.created_at,
    GREATEST(
        import_record.updated_at,
        COALESCE(job.updated_at, import_record.updated_at),
        COALESCE(run.updated_at, import_record.updated_at)
    )::timestamptz AS updated_at
FROM imports AS import_record
LEFT JOIN jobs AS job ON job.id = import_record.job_id
LEFT JOIN episodes AS episode
  ON episode.id = import_record.episode_id
 AND episode.user_id = import_record.user_id
LEFT JOIN LATERAL (
    SELECT candidate.*
    FROM transcription_runs AS candidate
    WHERE candidate.episode_id = import_record.episode_id
      AND candidate.user_id = import_record.user_id
    ORDER BY candidate.created_at DESC, candidate.id DESC
    LIMIT 1
) AS run ON true
LEFT JOIN transcript_versions AS transcript
  ON transcript.transcription_run_id = run.id
 AND transcript.user_id = import_record.user_id
WHERE import_record.id = sqlc.arg(import_id)
  AND import_record.user_id = sqlc.arg(user_id);

-- name: GetImportForResolve :one
SELECT import_record.*, job.status AS job_status
FROM imports AS import_record
JOIN jobs AS job ON job.id = import_record.job_id
WHERE import_record.id = sqlc.arg(import_id)
  AND import_record.user_id = sqlc.arg(user_id)
FOR UPDATE OF import_record;

-- name: CreateEpisode :one
INSERT INTO episodes (
    user_id,
    title,
    description,
    published_at,
    duration_ms,
    cover_url,
    resolve_status
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(title),
    sqlc.arg(description),
    sqlc.narg(published_at)::timestamptz,
    sqlc.arg(duration_ms),
    sqlc.arg(cover_url),
    'completed'
)
RETURNING *;

-- name: EnrichEpisode :one
UPDATE episodes
SET title = CASE WHEN episodes.title = '' THEN sqlc.arg(title) ELSE episodes.title END,
    description = CASE WHEN episodes.description = '' THEN sqlc.arg(description) ELSE episodes.description END,
    published_at = COALESCE(episodes.published_at, sqlc.narg(published_at)::timestamptz),
    duration_ms = CASE WHEN episodes.duration_ms = 0 THEN sqlc.arg(duration_ms) ELSE episodes.duration_ms END,
    cover_url = CASE WHEN episodes.cover_url = '' THEN sqlc.arg(cover_url) ELSE episodes.cover_url END,
    resolve_status = 'completed',
    updated_at = now()
WHERE episodes.id = sqlc.arg(episode_id)
  AND episodes.user_id = sqlc.arg(user_id)
RETURNING *;

-- name: AcquireEpisodeIdentityLock :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(identity_key), 0));

-- name: FindEpisodeByIdentityKeys :one
WITH requested AS (
    SELECT identity_key, ordinality
    FROM unnest(sqlc.arg(identity_keys)::text[]) WITH ORDINALITY AS input(identity_key, ordinality)
)
SELECT stored.episode_id
FROM requested
JOIN episode_identity_keys AS stored
  ON stored.user_id = sqlc.arg(user_id)
 AND stored.identity_key = requested.identity_key
ORDER BY requested.ordinality
LIMIT 1;

-- name: AddEpisodeIdentityKey :exec
INSERT INTO episode_identity_keys (user_id, identity_key, episode_id)
VALUES (sqlc.arg(user_id), sqlc.arg(identity_key), sqlc.arg(episode_id))
ON CONFLICT (user_id, identity_key) DO NOTHING;

-- name: AddEpisodeSource :exec
INSERT INTO episode_sources (
    user_id,
    episode_id,
    source_type,
    external_id,
    source_url,
    canonical_url,
    audio_url,
    download_headers,
    rss_guid
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(source_type),
    sqlc.narg(external_id)::text,
    sqlc.arg(source_url),
    sqlc.arg(canonical_url),
    sqlc.arg(audio_url),
    sqlc.arg(download_headers)::jsonb,
    sqlc.narg(rss_guid)::text
)
ON CONFLICT (user_id, episode_id, source_type, source_url) DO UPDATE
SET external_id = EXCLUDED.external_id,
    canonical_url = EXCLUDED.canonical_url,
    audio_url = EXCLUDED.audio_url,
    download_headers = EXCLUDED.download_headers,
    rss_guid = EXCLUDED.rss_guid,
    created_at = now();

-- name: SetImportEpisode :exec
UPDATE imports
SET episode_id = sqlc.arg(episode_id),
    updated_at = now()
WHERE id = sqlc.arg(import_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListTranscriptionTaskSegments :many
SELECT
    segment.start_ms,
    segment.text,
    speaker.display_name AS speaker_name
FROM transcript_segments AS segment
JOIN transcript_speakers AS speaker ON speaker.id = segment.speaker_id
JOIN transcript_versions AS transcript ON transcript.id = segment.transcript_version_id
JOIN imports AS import_record ON import_record.episode_id = transcript.episode_id
WHERE import_record.id = sqlc.arg(import_id)
  AND import_record.user_id = sqlc.arg(user_id)
  AND transcript.is_active
ORDER BY segment.sequence;
