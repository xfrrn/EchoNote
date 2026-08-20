-- name: AcquireNoteClientIDLock :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- name: GetNoteByClientID :one
SELECT *
FROM notes
WHERE user_id = sqlc.arg(user_id)
  AND client_note_id = sqlc.arg(client_note_id);

-- name: CreateNoteForEpisode :one
INSERT INTO notes (user_id, episode_id, client_note_id, content, created_at)
SELECT
    sqlc.arg(user_id),
    episode.id,
    sqlc.arg(client_note_id),
    sqlc.arg(content),
    sqlc.arg(created_at)
FROM episodes AS episode
WHERE episode.id = sqlc.arg(episode_id)
  AND episode.user_id = sqlc.arg(user_id)
RETURNING notes.*;

-- name: LockOwnedEpisodeForNote :one
SELECT id
FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
FOR KEY SHARE;

-- name: CreatePendingEpisode :one
INSERT INTO episodes (user_id, title, resolve_status)
VALUES (sqlc.arg(user_id), sqlc.arg(title), 'pending')
RETURNING *;

-- name: GetCaptureImport :one
SELECT import_record.id
FROM imports AS import_record
WHERE import_record.user_id = sqlc.arg(user_id)
  AND import_record.episode_id = sqlc.arg(episode_id)
  AND import_record.submitted_url = sqlc.arg(submitted_url)
ORDER BY import_record.created_at DESC, import_record.id DESC
LIMIT 1;

-- name: GetOwnedEpisodeID :one
SELECT id
FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListEpisodeNotes :many
SELECT *
FROM notes
WHERE episode_id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: UpdateNote :one
UPDATE notes
SET content = sqlc.arg(content),
    updated_at = now()
WHERE id = sqlc.arg(note_id)
  AND user_id = sqlc.arg(user_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: DeleteNote :one
UPDATE notes
SET deleted_at = COALESCE(deleted_at, now()),
    updated_at = CASE WHEN deleted_at IS NULL THEN now() ELSE updated_at END
WHERE id = sqlc.arg(note_id)
  AND user_id = sqlc.arg(user_id)
RETURNING id;

-- name: GetEpisodeForResolve :one
SELECT id, resolve_status
FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
FOR UPDATE;

-- name: ResolvePendingEpisode :one
UPDATE episodes
SET podcast_id = sqlc.narg(podcast_id)::uuid,
    title = sqlc.arg(title),
    description = sqlc.arg(description),
    published_at = sqlc.narg(published_at)::timestamptz,
    duration_ms = sqlc.arg(duration_ms),
    cover_url = sqlc.arg(cover_url),
    resolve_status = 'completed',
    updated_at = now()
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
  AND resolve_status = 'pending'
RETURNING *;

-- name: MoveEpisodeNotes :exec
UPDATE notes
SET episode_id = sqlc.arg(target_episode_id),
    updated_at = now()
WHERE episode_id = sqlc.arg(source_episode_id)
  AND user_id = sqlc.arg(user_id);

-- name: DeletePendingEpisode :exec
DELETE FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
  AND resolve_status = 'pending';
