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
    job.status,
    job.stage,
    job.error_code,
    job.error_message,
    import_record.created_at,
    GREATEST(import_record.updated_at, job.updated_at)::timestamptz AS updated_at
FROM imports AS import_record
JOIN jobs AS job ON job.id = import_record.job_id
WHERE import_record.id = sqlc.arg(import_id)
  AND import_record.user_id = sqlc.arg(user_id);

-- name: GetImportForResolve :one
SELECT import_record.*, job.status AS job_status
FROM imports AS import_record
JOIN jobs AS job ON job.id = import_record.job_id
WHERE import_record.id = sqlc.arg(import_id)
  AND import_record.user_id = sqlc.arg(user_id)
FOR UPDATE OF import_record;

-- name: UpsertPodcast :one
INSERT INTO podcasts (
    user_id,
    title,
    author,
    description,
    cover_url,
    feed_url
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(title),
    sqlc.arg(author),
    sqlc.arg(description),
    sqlc.arg(cover_url),
    sqlc.arg(feed_url)
)
ON CONFLICT (user_id, feed_url) WHERE feed_url IS NOT NULL
DO UPDATE SET
    title = EXCLUDED.title,
    author = CASE WHEN EXCLUDED.author <> '' THEN EXCLUDED.author ELSE podcasts.author END,
    description = CASE WHEN EXCLUDED.description <> '' THEN EXCLUDED.description ELSE podcasts.description END,
    cover_url = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE podcasts.cover_url END,
    updated_at = now()
RETURNING *;

-- name: CreateEpisode :one
INSERT INTO episodes (
    user_id,
    podcast_id,
    title,
    description,
    published_at,
    duration_ms,
    cover_url,
    resolve_status
) VALUES (
    sqlc.arg(user_id),
    sqlc.narg(podcast_id)::uuid,
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
SET podcast_id = COALESCE(episodes.podcast_id, sqlc.narg(podcast_id)::uuid),
    title = CASE
        WHEN episodes.podcast_id IS NULL AND sqlc.narg(podcast_id)::uuid IS NOT NULL
        THEN sqlc.arg(title)
        ELSE episodes.title
    END,
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
    rss_guid
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(source_type),
    sqlc.narg(external_id)::text,
    sqlc.arg(source_url),
    sqlc.arg(canonical_url),
    sqlc.arg(audio_url),
    sqlc.narg(rss_guid)::text
)
ON CONFLICT (user_id, episode_id, source_type, source_url) DO NOTHING;

-- name: SetImportEpisode :exec
UPDATE imports
SET episode_id = sqlc.arg(episode_id),
    updated_at = now()
WHERE id = sqlc.arg(import_id)
  AND user_id = sqlc.arg(user_id);
