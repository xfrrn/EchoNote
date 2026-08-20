BEGIN;

CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    type TEXT NOT NULL CHECK (btrim(type) <> ''),
    entity_type TEXT NOT NULL CHECK (btrim(entity_type) <> ''),
    entity_id UUID NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    stage TEXT NOT NULL DEFAULT 'queued' CHECK (btrim(stage) <> ''),
    priority SMALLINT NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_by TEXT,
    locked_at TIMESTAMPTZ,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CHECK (attempt <= max_attempts),
    CHECK (
        (status = 'running' AND locked_by IS NOT NULL AND locked_at IS NOT NULL)
        OR
        (status <> 'running' AND locked_by IS NULL AND locked_at IS NULL)
    ),
    CHECK (
        (status IN ('succeeded', 'failed', 'canceled')) = (completed_at IS NOT NULL)
    )
);

CREATE INDEX jobs_dequeue_idx
    ON jobs (priority DESC, run_after, created_at)
    WHERE status = 'queued';

CREATE INDEX jobs_stale_lock_idx
    ON jobs (locked_at)
    WHERE status = 'running';

CREATE TABLE job_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (btrim(event_type) <> ''),
    stage TEXT NOT NULL CHECK (btrim(stage) <> ''),
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX job_events_job_created_idx
    ON job_events (job_id, created_at, id);

COMMIT;
