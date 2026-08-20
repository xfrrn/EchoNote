-- name: LockOwnedEpisodeForSearch :one
SELECT id
FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
FOR UPDATE;

-- name: ListOwnedEpisodeIDsForSearch :many
SELECT id
FROM episodes
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at, id;

-- name: ListTranscriptSegmentsForSearch :many
SELECT
    segment.id,
    segment.speaker_id,
    segment.sequence,
    segment.start_ms,
    segment.end_ms,
    segment.text
FROM transcript_segments AS segment
JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
WHERE segment.transcript_version_id = sqlc.arg(transcript_id)
  AND version.user_id = sqlc.arg(user_id)
ORDER BY segment.sequence;

-- name: ListSearchDocumentsForEpisode :many
SELECT *
FROM search_documents
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND document_type IN ('note', 'transcript', 'ai_artifact')
ORDER BY document_type, source_id;

-- name: CreateSearchDocument :one
INSERT INTO search_documents (
    user_id,
    episode_id,
    document_type,
    source_id,
    content,
    content_hash,
    metadata
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(document_type),
    sqlc.arg(source_id),
    sqlc.arg(content),
    sqlc.arg(content_hash),
    sqlc.arg(metadata)::jsonb
)
RETURNING *;

-- name: UpdateSearchDocument :one
UPDATE search_documents
SET episode_id = sqlc.arg(episode_id),
    content = sqlc.arg(content),
    content_hash = sqlc.arg(content_hash),
    metadata = sqlc.arg(metadata)::jsonb,
    updated_at = now()
WHERE id = sqlc.arg(document_id)
  AND user_id = sqlc.arg(user_id)
RETURNING *;

-- name: DeleteSearchDocumentChunks :exec
DELETE FROM search_chunks
WHERE search_document_id = sqlc.arg(document_id);

-- name: CreateSearchChunk :exec
INSERT INTO search_chunks (
    search_document_id,
    chunk_index,
    text,
    start_ms,
    end_ms,
    speaker_id
) VALUES (
    sqlc.arg(document_id),
    sqlc.arg(chunk_index),
    sqlc.arg(text),
    sqlc.narg(start_ms)::bigint,
    sqlc.narg(end_ms)::bigint,
    sqlc.narg(speaker_id)::uuid
);

-- name: DeleteSearchDocument :exec
DELETE FROM search_documents
WHERE id = sqlc.arg(document_id)
  AND user_id = sqlc.arg(user_id);

-- name: CountPendingSearchEmbeddings :one
SELECT count(*)
FROM search_chunks AS chunk
JOIN search_documents AS document ON document.id = chunk.search_document_id
WHERE document.id = sqlc.arg(document_id)
  AND document.user_id = sqlc.arg(user_id)
  AND (chunk.embedding IS NULL OR chunk.embedding_model IS DISTINCT FROM sqlc.arg(embedding_model));

-- name: ListPendingSearchEmbeddings :many
SELECT chunk.id, chunk.text
FROM search_chunks AS chunk
JOIN search_documents AS document ON document.id = chunk.search_document_id
WHERE document.id = sqlc.arg(document_id)
  AND document.user_id = sqlc.arg(user_id)
  AND document.content_hash = sqlc.arg(content_hash)
  AND (chunk.embedding IS NULL OR chunk.embedding_model IS DISTINCT FROM sqlc.arg(embedding_model))
ORDER BY chunk.chunk_index
LIMIT sqlc.arg(batch_limit)::int;

-- name: SetSearchChunkEmbedding :execrows
UPDATE search_chunks AS chunk
SET embedding = sqlc.arg(embedding)::text::vector,
    embedding_model = sqlc.arg(embedding_model),
    updated_at = now()
FROM search_documents AS document
WHERE chunk.id = sqlc.arg(chunk_id)
  AND document.id = chunk.search_document_id
  AND document.user_id = sqlc.arg(user_id)
  AND document.content_hash = sqlc.arg(content_hash);

-- name: KeywordSearch :many
SELECT
    chunk.id AS chunk_id,
    document.document_type,
    document.source_id,
    document.episode_id,
    episode.title AS episode_title,
    COALESCE(podcast.title, '') AS podcast_title,
    chunk.speaker_id,
    COALESCE(speaker.display_name, '') AS speaker_name,
    chunk.start_ms,
    chunk.end_ms,
    chunk.text,
    CASE
        WHEN position(lower(sqlc.arg(query)) IN lower(chunk.text)) > 0 THEN 1.0
        ELSE word_similarity(lower(sqlc.arg(query)), lower(chunk.text))
    END::double precision AS rank_score
FROM search_chunks AS chunk
JOIN search_documents AS document ON document.id = chunk.search_document_id
JOIN episodes AS episode ON episode.id = document.episode_id AND episode.user_id = document.user_id
LEFT JOIN podcasts AS podcast ON podcast.id = episode.podcast_id AND podcast.user_id = episode.user_id
LEFT JOIN transcript_speakers AS speaker ON speaker.id = chunk.speaker_id
WHERE document.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(episode_id)::uuid IS NULL OR document.episode_id = sqlc.narg(episode_id)::uuid)
  AND (
      position(lower(sqlc.arg(query)) IN lower(chunk.text)) > 0
      OR word_similarity(lower(sqlc.arg(query)), lower(chunk.text)) >= 0.2
  )
ORDER BY rank_score DESC, document.updated_at DESC, chunk.id
LIMIT sqlc.arg(candidate_limit)::int;

-- name: SemanticSearch :many
SELECT
    chunk.id AS chunk_id,
    document.document_type,
    document.source_id,
    document.episode_id,
    episode.title AS episode_title,
    COALESCE(podcast.title, '') AS podcast_title,
    chunk.speaker_id,
    COALESCE(speaker.display_name, '') AS speaker_name,
    chunk.start_ms,
    chunk.end_ms,
    chunk.text,
    (1 - (chunk.embedding <=> sqlc.arg(query_embedding)::text::vector))::double precision AS rank_score
FROM search_chunks AS chunk
JOIN search_documents AS document ON document.id = chunk.search_document_id
JOIN episodes AS episode ON episode.id = document.episode_id AND episode.user_id = document.user_id
LEFT JOIN podcasts AS podcast ON podcast.id = episode.podcast_id AND podcast.user_id = episode.user_id
LEFT JOIN transcript_speakers AS speaker ON speaker.id = chunk.speaker_id
WHERE document.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(episode_id)::uuid IS NULL OR document.episode_id = sqlc.narg(episode_id)::uuid)
  AND chunk.embedding IS NOT NULL
  AND chunk.embedding_model = sqlc.arg(embedding_model)
ORDER BY chunk.embedding <=> sqlc.arg(query_embedding)::text::vector, chunk.id
LIMIT sqlc.arg(candidate_limit)::int;

-- name: CancelEpisodeSearchJobs :many
UPDATE jobs AS job
SET status = 'canceled',
    stage = 'canceled',
    locked_by = NULL,
    locked_at = NULL,
    error_code = NULL,
    error_message = NULL,
    updated_at = now(),
    completed_at = now()
WHERE job.user_id = sqlc.arg(user_id)
  AND job.status IN ('queued', 'running')
  AND (
      (job.type = 'build_keyword_index' AND job.entity_type = 'episode' AND job.entity_id = sqlc.arg(episode_id))
      OR
      (job.type = 'generate_embeddings' AND job.entity_type = 'search_document' AND job.entity_id IN (
          SELECT document.id
          FROM search_documents AS document
          WHERE document.user_id = sqlc.arg(user_id)
            AND document.episode_id = sqlc.arg(episode_id)
      ))
  )
RETURNING job.*;
