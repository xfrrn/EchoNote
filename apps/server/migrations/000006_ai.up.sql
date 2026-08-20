BEGIN;

CREATE TABLE ai_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    transcript_version_id UUID NOT NULL REFERENCES transcript_versions(id) ON DELETE CASCADE,
    job_id UUID UNIQUE REFERENCES jobs(id) ON DELETE SET NULL,
    artifact_type TEXT NOT NULL CHECK (artifact_type = 'episode_summary'),
    model TEXT NOT NULL CHECK (btrim(model) <> ''),
    prompt_version TEXT NOT NULL CHECK (btrim(prompt_version) <> ''),
    notes_revision TEXT NOT NULL CHECK (notes_revision ~ '^[0-9a-f]{64}$'),
    input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'generating', 'ready', 'stale', 'failed')),
    result JSONB,
    search_text TEXT NOT NULL DEFAULT '',
    error_code TEXT,
    error_message TEXT,
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (
        user_id,
        episode_id,
        artifact_type,
        transcript_version_id,
        notes_revision,
        input_hash,
        model,
        prompt_version
    ),
    CHECK (status <> 'ready' OR (result IS NOT NULL AND btrim(search_text) <> '' AND completed_at IS NOT NULL))
);

CREATE UNIQUE INDEX ai_artifacts_current_idx
    ON ai_artifacts (user_id, episode_id, artifact_type)
    WHERE status IN ('queued', 'generating', 'ready');

CREATE INDEX ai_artifacts_episode_created_idx
    ON ai_artifacts (user_id, episode_id, created_at DESC, id DESC);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID REFERENCES episodes(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('episode', 'library')),
    title TEXT NOT NULL CHECK (btrim(title) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scope = 'episode' AND episode_id IS NOT NULL)
        OR
        (scope = 'library' AND episode_id IS NULL)
    )
);

CREATE INDEX conversations_user_updated_idx
    ON conversations (user_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    reply_to_message_id UUID UNIQUE REFERENCES messages(id) ON DELETE CASCADE,
    client_message_id UUID,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    status TEXT NOT NULL CHECK (status IN ('streaming', 'completed', 'failed')),
    content TEXT NOT NULL DEFAULT '',
    model TEXT,
    error_code TEXT,
    error_message TEXT,
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CHECK (
        (role = 'user' AND client_message_id IS NOT NULL AND reply_to_message_id IS NULL AND status = 'completed' AND model IS NULL)
        OR
        (role = 'assistant' AND client_message_id IS NULL AND reply_to_message_id IS NOT NULL AND model IS NOT NULL)
    ),
    CHECK (status <> 'completed' OR (btrim(content) <> '' AND completed_at IS NOT NULL)),
    CHECK (status <> 'failed' OR completed_at IS NOT NULL),
    UNIQUE (conversation_id, client_message_id)
);

CREATE INDEX messages_conversation_created_idx
    ON messages (conversation_id, created_at, id);

CREATE TABLE message_citations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    transcript_segment_id UUID REFERENCES transcript_segments(id) ON DELETE CASCADE,
    note_id UUID REFERENCES notes(id) ON DELETE CASCADE,
    excerpt TEXT NOT NULL CHECK (btrim(excerpt) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(transcript_segment_id, note_id) = 1),
    UNIQUE (message_id, position)
);

CREATE INDEX message_citations_message_idx
    ON message_citations (message_id, position);

COMMIT;
