-- name: LockOwnedEpisodeForAI :one
SELECT id, title
FROM episodes
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id)
FOR UPDATE;

-- name: SetEpisodeAIStatus :exec
UPDATE episodes
SET ai_status = sqlc.arg(status),
    updated_at = now()
WHERE id = sqlc.arg(episode_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetOwnedNoteEpisodeForAI :one
SELECT episode_id
FROM notes
WHERE id = sqlc.arg(note_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetCachedAIArtifact :one
SELECT *
FROM ai_artifacts
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND artifact_type = sqlc.arg(artifact_type)
  AND transcript_version_id = sqlc.arg(transcript_version_id)
  AND notes_revision = sqlc.arg(notes_revision)
  AND input_hash = sqlc.arg(input_hash)
  AND model = sqlc.arg(model)
  AND prompt_version = sqlc.arg(prompt_version);

-- name: CreateAIArtifact :one
INSERT INTO ai_artifacts (
    user_id,
    episode_id,
    transcript_version_id,
    artifact_type,
    model,
    prompt_version,
    notes_revision,
    input_hash
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(episode_id),
    sqlc.arg(transcript_version_id),
    sqlc.arg(artifact_type),
    sqlc.arg(model),
    sqlc.arg(prompt_version),
    sqlc.arg(notes_revision),
    sqlc.arg(input_hash)
)
RETURNING *;

-- name: ResetAIArtifactForGeneration :one
UPDATE ai_artifacts
SET status = 'queued',
    job_id = NULL,
    result = NULL,
    search_text = '',
    error_code = NULL,
    error_message = NULL,
    input_tokens = 0,
    output_tokens = 0,
    completed_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND status IN ('failed', 'stale')
RETURNING *;

-- name: ReactivateCachedAIArtifact :one
UPDATE ai_artifacts
SET status = 'ready',
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'stale'
  AND result IS NOT NULL
  AND btrim(search_text) <> ''
RETURNING *;

-- name: MarkOtherAIArtifactsStale :exec
UPDATE ai_artifacts
SET status = 'stale',
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND artifact_type = sqlc.arg(artifact_type)
  AND status IN ('queued', 'generating', 'ready')
  AND (sqlc.narg(except_artifact_id)::uuid IS NULL OR id <> sqlc.narg(except_artifact_id)::uuid);

-- name: MarkEpisodeAIArtifactsStale :execrows
UPDATE ai_artifacts
SET status = 'stale',
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND status IN ('queued', 'generating', 'ready');

-- name: CancelEpisodeAIJobsExcept :many
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
  AND job.type = 'generate_ai_artifact'
  AND job.status IN ('queued', 'running')
  AND job.entity_type = 'ai_artifact'
  AND job.entity_id IN (
      SELECT artifact.id
      FROM ai_artifacts AS artifact
      WHERE artifact.user_id = sqlc.arg(user_id)
        AND artifact.episode_id = sqlc.arg(episode_id)
        AND (sqlc.narg(except_artifact_id)::uuid IS NULL OR artifact.id <> sqlc.narg(except_artifact_id)::uuid)
  )
RETURNING job.*;

-- name: AttachAIArtifactJob :one
UPDATE ai_artifacts
SET job_id = sqlc.arg(job_id),
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'queued'
RETURNING *;

-- name: GetAIArtifactForJob :one
SELECT *
FROM ai_artifacts
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND job_id = sqlc.arg(job_id);

-- name: StartAIArtifactGeneration :one
UPDATE ai_artifacts
SET status = 'generating',
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND job_id = sqlc.arg(job_id)
  AND status IN ('queued', 'generating')
RETURNING *;

-- name: MarkAIArtifactStale :exec
UPDATE ai_artifacts
SET status = 'stale',
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND status IN ('queued', 'generating', 'ready');

-- name: CompleteAIArtifact :one
UPDATE ai_artifacts
SET status = 'ready',
    result = sqlc.arg(result)::jsonb,
    search_text = sqlc.arg(search_text),
    error_code = NULL,
    error_message = NULL,
    input_tokens = sqlc.arg(input_tokens),
    output_tokens = sqlc.arg(output_tokens),
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND input_hash = sqlc.arg(input_hash)
  AND status = 'generating'
RETURNING *;

-- name: FailAIArtifact :one
UPDATE ai_artifacts
SET status = 'failed',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(artifact_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'generating'
RETURNING *;

-- name: ListEpisodeAIArtifacts :many
SELECT *
FROM ai_artifacts
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: ReconcileFailedAIArtifacts :execrows
UPDATE ai_artifacts AS artifact
SET status = 'failed',
    error_code = COALESCE(job.error_code, 'AI_JOB_FAILED'),
    error_message = COALESCE(job.error_message, 'AI generation job failed'),
    completed_at = COALESCE(job.completed_at, now()),
    updated_at = now()
FROM jobs AS job
WHERE artifact.user_id = sqlc.arg(user_id)
  AND artifact.episode_id = sqlc.arg(episode_id)
  AND artifact.status IN ('queued', 'generating')
  AND job.id = artifact.job_id
  AND job.status = 'failed';

-- name: GetReadyAIArtifactForSearch :one
SELECT id, search_text
FROM ai_artifacts
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND artifact_type = 'episode_summary'
  AND status = 'ready'
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: CreateConversation :one
INSERT INTO conversations (user_id, episode_id, scope, title)
VALUES (sqlc.arg(user_id), sqlc.narg(episode_id)::uuid, sqlc.arg(scope), sqlc.arg(title))
RETURNING *;

-- name: GetOwnedConversation :one
SELECT conversation.*, episode.title AS episode_title
FROM conversations AS conversation
LEFT JOIN episodes AS episode
    ON episode.id = conversation.episode_id
   AND episode.user_id = conversation.user_id
WHERE conversation.id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id);

-- name: LockOwnedConversation :one
SELECT conversation.*, episode.title AS episode_title
FROM conversations AS conversation
LEFT JOIN episodes AS episode
    ON episode.id = conversation.episode_id
   AND episode.user_id = conversation.user_id
WHERE conversation.id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id)
FOR UPDATE OF conversation;

-- name: TouchConversation :exec
UPDATE conversations
SET updated_at = now()
WHERE id = sqlc.arg(conversation_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetUserMessageByClientID :one
SELECT message.*
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
WHERE message.conversation_id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id)
  AND message.client_message_id = sqlc.arg(client_message_id)
  AND message.role = 'user';

-- name: GetAssistantReply :one
SELECT message.*
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
WHERE message.reply_to_message_id = sqlc.arg(user_message_id)
  AND conversation.user_id = sqlc.arg(user_id)
  AND message.role = 'assistant';

-- name: CreateUserMessage :one
INSERT INTO messages (
    conversation_id, client_message_id, role, status, content, completed_at
) VALUES (
    sqlc.arg(conversation_id), sqlc.arg(client_message_id), 'user', 'completed', sqlc.arg(content), now()
)
RETURNING *;

-- name: CreateAssistantMessage :one
INSERT INTO messages (
    conversation_id, reply_to_message_id, role, status, model
) VALUES (
    sqlc.arg(conversation_id), sqlc.arg(user_message_id), 'assistant', 'streaming', sqlc.arg(model)
)
RETURNING *;

-- name: CompleteAssistantMessage :one
UPDATE messages AS message
SET status = 'completed',
    content = sqlc.arg(content),
    input_tokens = sqlc.arg(input_tokens),
    output_tokens = sqlc.arg(output_tokens),
    error_code = NULL,
    error_message = NULL,
    completed_at = now(),
    updated_at = now()
FROM conversations AS conversation
WHERE message.id = sqlc.arg(message_id)
  AND conversation.id = message.conversation_id
  AND conversation.user_id = sqlc.arg(user_id)
  AND message.status = 'streaming'
RETURNING message.*;

-- name: FailAssistantMessage :execrows
UPDATE messages AS message
SET status = 'failed',
    content = '',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    completed_at = now(),
    updated_at = now()
FROM conversations AS conversation
WHERE message.id = sqlc.arg(message_id)
  AND conversation.id = message.conversation_id
  AND conversation.user_id = sqlc.arg(user_id)
  AND message.status = 'streaming';

-- name: CreateMessageCitation :one
INSERT INTO message_citations (
    message_id, position, transcript_segment_id, note_id, excerpt
) VALUES (
    sqlc.arg(message_id),
    sqlc.arg(position),
    sqlc.narg(transcript_segment_id)::uuid,
    sqlc.narg(note_id)::uuid,
    sqlc.arg(excerpt)
)
RETURNING *;

-- name: ListConversationMessages :many
SELECT message.*
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
LEFT JOIN messages AS parent ON parent.id = message.reply_to_message_id
WHERE message.conversation_id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id)
ORDER BY
    COALESCE(parent.created_at, message.created_at),
    CASE WHEN message.role = 'user' THEN 0 ELSE 1 END,
    message.id;

-- name: ListConversationHistory :many
SELECT message.role, message.content
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
LEFT JOIN messages AS parent ON parent.id = message.reply_to_message_id
WHERE message.conversation_id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id)
  AND message.status = 'completed'
  AND message.id <> sqlc.arg(current_user_message_id)
  AND (
      message.role = 'assistant'
      OR EXISTS (
          SELECT 1
          FROM messages AS reply
          WHERE reply.reply_to_message_id = message.id
            AND reply.role = 'assistant'
            AND reply.status = 'completed'
      )
  )
ORDER BY
    COALESCE(parent.created_at, message.created_at) DESC,
    CASE WHEN message.role = 'assistant' THEN 1 ELSE 0 END DESC,
    message.id DESC
LIMIT 20;

-- name: ListConversationCitations :many
SELECT
    citation.*,
    COALESCE(speaker.id, '00000000-0000-0000-0000-000000000000'::uuid) AS speaker_id,
    COALESCE(speaker.display_name, '') AS speaker_name,
    segment.start_ms,
    segment.end_ms
FROM message_citations AS citation
JOIN messages AS message ON message.id = citation.message_id
JOIN conversations AS conversation ON conversation.id = message.conversation_id
LEFT JOIN transcript_segments AS segment ON segment.id = citation.transcript_segment_id
LEFT JOIN transcript_speakers AS speaker ON speaker.id = segment.speaker_id
WHERE conversation.id = sqlc.arg(conversation_id)
  AND conversation.user_id = sqlc.arg(user_id)
ORDER BY message.created_at, citation.position;

-- name: ListAISegmentsForSearchChunks :many
SELECT
    chunk.id AS search_chunk_id,
    segment.id,
    segment.speaker_id,
    speaker.display_name AS speaker_name,
    segment.start_ms,
    segment.end_ms,
    segment.text,
    segment.sequence
FROM search_chunks AS chunk
JOIN search_documents AS document ON document.id = chunk.search_document_id
JOIN transcript_segments AS segment
    ON segment.transcript_version_id = document.source_id
   AND segment.start_ms >= chunk.start_ms
   AND segment.end_ms <= chunk.end_ms
   AND segment.speaker_id = chunk.speaker_id
JOIN transcript_speakers AS speaker ON speaker.id = segment.speaker_id
WHERE chunk.id = ANY(sqlc.arg(search_chunk_ids)::uuid[])
  AND document.user_id = sqlc.arg(user_id)
  AND document.episode_id = sqlc.arg(episode_id)
  AND document.document_type = 'transcript'
ORDER BY chunk.chunk_index, segment.sequence;

-- name: ListAINotesByIDs :many
SELECT note.*
FROM notes AS note
WHERE note.id = ANY(sqlc.arg(note_ids)::uuid[])
  AND note.user_id = sqlc.arg(user_id)
  AND note.episode_id = sqlc.arg(episode_id)
  AND note.deleted_at IS NULL
ORDER BY note.created_at, note.id;

-- name: ListFallbackAISegments :many
SELECT
    segment.id,
    segment.speaker_id,
    speaker.display_name AS speaker_name,
    segment.start_ms,
    segment.end_ms,
    segment.text,
    segment.sequence
FROM transcript_segments AS segment
JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
JOIN transcript_speakers AS speaker ON speaker.id = segment.speaker_id
WHERE version.user_id = sqlc.arg(user_id)
  AND version.episode_id = sqlc.arg(episode_id)
  AND version.is_active
ORDER BY segment.sequence
LIMIT sqlc.arg(candidate_limit)::int;

-- name: ListFallbackAINotes :many
SELECT note.*
FROM notes AS note
WHERE note.user_id = sqlc.arg(user_id)
  AND note.episode_id = sqlc.arg(episode_id)
  AND note.deleted_at IS NULL
ORDER BY note.created_at DESC, note.id DESC
LIMIT sqlc.arg(candidate_limit)::int;
