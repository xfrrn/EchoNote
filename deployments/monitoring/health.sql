\set ON_ERROR_STOP on
\pset pager off

-- Queue age and depth. Alert thresholds live in alerts.md.
SELECT type,
       count(*) AS queued,
       round(extract(epoch FROM (now() - min(run_after))) / 60, 1) AS oldest_minutes
FROM jobs
WHERE status = 'queued' AND run_after <= now()
GROUP BY type
ORDER BY oldest_minutes DESC;

-- Failed jobs expose identifiers and codes, never payload or error_message.
SELECT type, error_code, count(*) AS failures, max(completed_at) AS last_failure
FROM jobs
WHERE status = 'failed' AND completed_at >= now() - interval '24 hours'
GROUP BY type, error_code
ORDER BY failures DESC, type;

SELECT id AS cleanup_job_id, user_id, entity_id, error_code, completed_at
FROM jobs
WHERE type = 'cleanup_audio' AND status = 'failed'
ORDER BY completed_at DESC;

-- Default lease is five minutes; pass a platform-specific threshold if changed.
SELECT type, count(*) AS stale_running, min(locked_at) AS oldest_lock
FROM jobs
WHERE status = 'running' AND locked_at < now() - interval '5 minutes'
GROUP BY type;

SELECT count(*) AS connections,
       current_setting('max_connections')::integer AS max_connections,
       round(100.0 * count(*) / current_setting('max_connections')::integer, 1) AS percent_used
FROM pg_stat_activity;

-- Dry-run parity for the automatic retention policy.
SELECT
    (SELECT count(*) FROM sessions
     WHERE expires_at < now() - interval '30 days'
        OR revoked_at < now() - interval '30 days') AS expired_sessions,
    (SELECT count(*) FROM jobs
     WHERE status IN ('succeeded', 'canceled')
       AND completed_at < now() - interval '30 days') AS completed_jobs,
    (SELECT count(*) FROM jobs
     WHERE status = 'failed'
       AND completed_at < now() - interval '90 days') AS failed_jobs;

-- Daily cost units; compare with the target account's configured budgets.
WITH daily_usage (unit, provider, model, value) AS (
    SELECT 'asr_audio_seconds', provider, model,
           sum(chunk.render_end_ms - chunk.render_start_ms) / 1000.0
    FROM transcription_chunks AS chunk
    JOIN transcription_runs AS run ON run.id = chunk.transcription_run_id
    WHERE chunk.external_task_id IS NOT NULL
      AND chunk.created_at >= date_trunc('day', now())
    GROUP BY provider, model
    UNION ALL
    SELECT 'llm_input_tokens', 'aliyun_llm', model, sum(input_tokens)::numeric
    FROM ai_artifacts
    WHERE created_at >= date_trunc('day', now())
    GROUP BY model
    UNION ALL
    SELECT 'llm_output_tokens', 'aliyun_llm', model, sum(output_tokens)::numeric
    FROM ai_artifacts
    WHERE created_at >= date_trunc('day', now())
    GROUP BY model
    UNION ALL
    SELECT 'llm_input_tokens', 'aliyun_llm', model, sum(input_tokens)::numeric
    FROM messages
    WHERE model IS NOT NULL AND created_at >= date_trunc('day', now())
    GROUP BY model
    UNION ALL
    SELECT 'llm_output_tokens', 'aliyun_llm', model, sum(output_tokens)::numeric
    FROM messages
    WHERE model IS NOT NULL AND created_at >= date_trunc('day', now())
    GROUP BY model
    UNION ALL
    SELECT 'embedding_chunks', 'aliyun_embedding', embedding_model, count(*)::numeric
    FROM search_chunks
    WHERE embedding IS NOT NULL AND updated_at >= date_trunc('day', now())
    GROUP BY embedding_model
)
SELECT unit, provider, model, round(sum(value), 1) AS value
FROM daily_usage
GROUP BY unit, provider, model
ORDER BY unit, provider, model;
