BEGIN;

CREATE TABLE search_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    document_type TEXT NOT NULL CHECK (document_type IN ('note', 'transcript', 'ai_artifact')),
    source_id UUID NOT NULL,
    content TEXT NOT NULL CHECK (btrim(content) <> ''),
    content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, document_type, source_id)
);

CREATE INDEX search_documents_user_episode_idx
    ON search_documents (user_id, episode_id, document_type, updated_at DESC);

CREATE TABLE search_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    search_document_id UUID NOT NULL REFERENCES search_documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
    text TEXT NOT NULL CHECK (btrim(text) <> ''),
    start_ms BIGINT,
    end_ms BIGINT,
    speaker_id UUID REFERENCES transcript_speakers(id) ON DELETE SET NULL,
    embedding TEXT,
    embedding_model TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (search_document_id, chunk_index),
    CHECK (
        (start_ms IS NULL AND end_ms IS NULL)
        OR
        (start_ms IS NOT NULL AND start_ms >= 0 AND end_ms IS NOT NULL AND end_ms > start_ms)
    ),
    CHECK ((embedding IS NULL) = (embedding_model IS NULL))
);

CREATE INDEX search_chunks_document_idx
    ON search_chunks (search_document_id, chunk_index);

WITH inserted_jobs AS (
    INSERT INTO jobs (
        user_id, type, entity_type, entity_id, payload, stage, priority, max_attempts
    )
    SELECT
        episode.user_id,
        'build_keyword_index',
        'episode',
        episode.id,
        '{}'::jsonb,
        'queued',
        0,
        3
    FROM episodes AS episode
    RETURNING id, stage
)
INSERT INTO job_events (job_id, event_type, stage, data)
SELECT id, 'queued', stage, '{}'::jsonb
FROM inserted_jobs;

COMMIT;
