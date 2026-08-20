-- name: GetReadyAIArtifactForExport :one
SELECT result
FROM ai_artifacts
WHERE user_id = sqlc.arg(user_id)
  AND episode_id = sqlc.arg(episode_id)
  AND artifact_type = 'episode_summary'
  AND status = 'ready'
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: GetTranscriptExportStats :one
SELECT
    count(*)::int AS segment_count,
    COALESCE(sum(octet_length(candidate.text)), 0)::bigint AS content_bytes
FROM (
    SELECT segment.text
    FROM transcript_segments AS segment
    JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
    WHERE segment.transcript_version_id = sqlc.arg(transcript_id)
      AND version.user_id = sqlc.arg(user_id)
      AND (
          cardinality(sqlc.arg(segment_ids)::uuid[]) = 0
          OR segment.id = ANY(sqlc.arg(segment_ids)::uuid[])
      )
    ORDER BY segment.sequence
    LIMIT sqlc.arg(segment_limit)::int
) AS candidate;

-- name: ListTranscriptSegmentsForExport :many
SELECT
    segment.id,
    speaker.display_name AS speaker_name,
    segment.start_ms,
    segment.text
FROM transcript_segments AS segment
JOIN transcript_versions AS version ON version.id = segment.transcript_version_id
JOIN transcript_speakers AS speaker ON speaker.id = segment.speaker_id
WHERE segment.transcript_version_id = sqlc.arg(transcript_id)
  AND version.user_id = sqlc.arg(user_id)
  AND (
      cardinality(sqlc.arg(segment_ids)::uuid[]) = 0
      OR segment.id = ANY(sqlc.arg(segment_ids)::uuid[])
  )
ORDER BY segment.sequence
LIMIT sqlc.arg(segment_limit)::int;
