-- name: ListLibraryEpisodes :many
SELECT
    episode.id,
    episode.title,
    episode.published_at,
    episode.duration_ms,
    episode.cover_url,
    episode.resolve_status,
    episode.transcription_status,
    episode.ai_status,
    episode.created_at,
    episode.updated_at,
    podcast.id AS podcast_id,
    podcast.title AS podcast_title,
    podcast.author AS podcast_author,
    podcast.description AS podcast_description,
    podcast.cover_url AS podcast_cover_url,
    podcast.feed_url AS podcast_feed_url,
    (
        SELECT count(*)
        FROM episode_sources AS source
        WHERE source.episode_id = episode.id
          AND source.user_id = episode.user_id
    )::bigint AS source_count
FROM episodes AS episode
LEFT JOIN podcasts AS podcast
  ON podcast.id = episode.podcast_id
 AND podcast.user_id = episode.user_id
WHERE episode.user_id = sqlc.arg(user_id)
ORDER BY episode.created_at DESC, episode.id DESC
LIMIT sqlc.arg(page_limit)::int
OFFSET sqlc.arg(page_offset)::int;

-- name: CountLibraryEpisodes :one
SELECT count(*)
FROM episodes
WHERE user_id = sqlc.arg(user_id);

-- name: GetLibraryEpisode :one
SELECT
    episode.id,
    episode.title,
    episode.description,
    episode.published_at,
    episode.duration_ms,
    episode.cover_url,
    episode.resolve_status,
    episode.transcription_status,
    episode.ai_status,
    episode.created_at,
    episode.updated_at,
    podcast.id AS podcast_id,
    podcast.title AS podcast_title,
    podcast.author AS podcast_author,
    podcast.description AS podcast_description,
    podcast.cover_url AS podcast_cover_url,
    podcast.feed_url AS podcast_feed_url,
    (
        SELECT count(*)
        FROM episode_sources AS source
        WHERE source.episode_id = episode.id
          AND source.user_id = episode.user_id
    )::bigint AS source_count
FROM episodes AS episode
LEFT JOIN podcasts AS podcast
  ON podcast.id = episode.podcast_id
 AND podcast.user_id = episode.user_id
WHERE episode.id = sqlc.arg(episode_id)
  AND episode.user_id = sqlc.arg(user_id);

-- name: ListLibraryEpisodeSources :many
SELECT
    id,
    source_type,
    external_id,
    source_url,
    canonical_url,
    rss_guid,
    created_at
FROM episode_sources
WHERE episode_id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
ORDER BY created_at, id;

-- name: DeleteLibraryEpisode :one
DELETE FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
RETURNING podcast_id;

-- name: DeleteOrphanPodcast :exec
DELETE FROM podcasts AS podcast
WHERE podcast.id = sqlc.arg(podcast_id)
  AND podcast.user_id = sqlc.arg(user_id)
  AND NOT EXISTS (
      SELECT 1
      FROM episodes AS episode
      WHERE episode.podcast_id = podcast.id
        AND episode.user_id = podcast.user_id
  );
