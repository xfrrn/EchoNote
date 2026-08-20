-- name: EnqueueJob :one
INSERT INTO jobs (
    user_id,
    type,
    entity_type,
    entity_id,
    payload,
    stage,
    priority,
    max_attempts,
    run_after
) VALUES (
    sqlc.narg(user_id)::uuid,
    sqlc.arg(job_type),
    sqlc.arg(entity_type),
    sqlc.arg(entity_id),
    sqlc.arg(payload)::jsonb,
    sqlc.arg(stage),
    sqlc.arg(priority),
    sqlc.arg(max_attempts),
    sqlc.arg(run_after)
)
RETURNING *;

-- name: ClaimJob :one
WITH next_job AS (
    SELECT id
    FROM jobs
    WHERE status = 'queued'
      AND run_after <= now()
      AND attempt < max_attempts
      AND type = ANY(sqlc.arg(job_types)::text[])
    ORDER BY priority DESC, run_after, created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs AS job
SET status = 'running',
    stage = 'running',
    attempt = job.attempt + 1,
    locked_by = sqlc.arg(worker_id),
    locked_at = now(),
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
FROM next_job
WHERE job.id = next_job.id
RETURNING job.*;

-- name: HeartbeatJob :execrows
UPDATE jobs
SET locked_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(job_id)
  AND status = 'running'
  AND locked_by = sqlc.arg(worker_id);

-- name: CompleteJob :one
UPDATE jobs
SET status = 'succeeded',
    stage = 'completed',
    locked_by = NULL,
    locked_at = NULL,
    error_code = NULL,
    error_message = NULL,
    updated_at = now(),
    completed_at = now()
WHERE id = sqlc.arg(job_id)
  AND status = 'running'
  AND locked_by = sqlc.arg(worker_id)
RETURNING *;

-- name: RetryOrFailJob :one
UPDATE jobs
SET status = CASE WHEN sqlc.arg(retryable)::boolean AND attempt < max_attempts THEN 'queued' ELSE 'failed' END,
    stage = CASE WHEN sqlc.arg(retryable)::boolean AND attempt < max_attempts THEN 'retry_wait' ELSE 'failed' END,
    run_after = CASE
        WHEN sqlc.arg(retryable)::boolean AND attempt < max_attempts
        THEN now() + (sqlc.arg(retry_delay_milliseconds)::bigint * interval '1 millisecond')
        ELSE run_after
    END,
    locked_by = NULL,
    locked_at = NULL,
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    updated_at = now(),
    completed_at = CASE WHEN sqlc.arg(retryable)::boolean AND attempt < max_attempts THEN NULL ELSE now() END
WHERE id = sqlc.arg(job_id)
  AND status = 'running'
  AND locked_by = sqlc.arg(worker_id)
RETURNING *;

-- name: RescheduleJob :one
UPDATE jobs
SET status = 'queued',
    stage = 'waiting_external',
    attempt = GREATEST(attempt - 1, 0),
    run_after = now() + (sqlc.arg(delay_milliseconds)::bigint * interval '1 millisecond'),
    locked_by = NULL,
    locked_at = NULL,
    error_code = NULL,
    error_message = NULL,
    updated_at = now(),
    completed_at = NULL
WHERE id = sqlc.arg(job_id)
  AND status = 'running'
  AND locked_by = sqlc.arg(worker_id)
RETURNING *;

-- name: RequeueStaleJobs :many
UPDATE jobs
SET status = CASE WHEN attempt < max_attempts THEN 'queued' ELSE 'failed' END,
    stage = CASE WHEN attempt < max_attempts THEN 'retry_wait' ELSE 'failed' END,
    run_after = CASE WHEN attempt < max_attempts THEN now() ELSE run_after END,
    locked_by = NULL,
    locked_at = NULL,
    error_code = 'JOB_LEASE_EXPIRED',
    error_message = 'worker lease expired before completion',
    updated_at = now(),
    completed_at = CASE WHEN attempt < max_attempts THEN NULL ELSE now() END
WHERE status = 'running'
  AND locked_at < now() - (sqlc.arg(lease_milliseconds)::bigint * interval '1 millisecond')
RETURNING *;

-- name: CreateJobEvent :one
INSERT INTO job_events (job_id, event_type, stage, data)
VALUES (
    sqlc.arg(job_id),
    sqlc.arg(event_type),
    sqlc.arg(stage),
    sqlc.arg(data)::jsonb
)
RETURNING *;

-- name: GetJob :one
SELECT *
FROM jobs
WHERE id = sqlc.arg(job_id);

-- name: ListJobEvents :many
SELECT *
FROM job_events
WHERE job_id = sqlc.arg(job_id)
ORDER BY created_at, id;
