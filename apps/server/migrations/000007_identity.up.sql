BEGIN;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT,
    username_normalized TEXT,
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'placeholder'
        CHECK (status IN ('placeholder', 'active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((username IS NULL) = (username_normalized IS NULL)),
    CHECK (username IS NULL OR btrim(username) <> ''),
    CHECK (username_normalized IS NULL OR btrim(username_normalized) <> ''),
    CHECK (password_hash IS NULL OR btrim(password_hash) <> ''),
    CHECK (status <> 'active' OR (username IS NOT NULL AND password_hash IS NOT NULL)),
    CHECK (status <> 'placeholder' OR (username IS NULL AND password_hash IS NULL))
);

CREATE UNIQUE INDEX users_username_normalized_idx
    ON users (username_normalized)
    WHERE username_normalized IS NOT NULL;

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK (last_seen_at >= created_at)
);

CREATE INDEX sessions_user_active_idx
    ON sessions (user_id, expires_at)
    WHERE revoked_at IS NULL;

INSERT INTO users (id)
SELECT DISTINCT user_id
FROM (
    SELECT user_id FROM jobs
    UNION ALL SELECT user_id FROM podcasts
    UNION ALL SELECT user_id FROM episodes
    UNION ALL SELECT user_id FROM episode_sources
    UNION ALL SELECT user_id FROM episode_identity_keys
    UNION ALL SELECT user_id FROM imports
    UNION ALL SELECT user_id FROM notes
    UNION ALL SELECT user_id FROM transcription_runs
    UNION ALL SELECT user_id FROM transcript_versions
    UNION ALL SELECT user_id FROM search_documents
    UNION ALL SELECT user_id FROM ai_artifacts
    UNION ALL SELECT user_id FROM conversations
) AS existing_users
WHERE user_id IS NOT NULL
ON CONFLICT (id) DO NOTHING;

ALTER TABLE jobs ADD CONSTRAINT jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE podcasts ADD CONSTRAINT podcasts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE episodes ADD CONSTRAINT episodes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE episode_sources ADD CONSTRAINT episode_sources_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE episode_identity_keys ADD CONSTRAINT episode_identity_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE imports ADD CONSTRAINT imports_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE notes ADD CONSTRAINT notes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE transcription_runs ADD CONSTRAINT transcription_runs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE transcript_versions ADD CONSTRAINT transcript_versions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE search_documents ADD CONSTRAINT search_documents_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE ai_artifacts ADD CONSTRAINT ai_artifacts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;
ALTER TABLE conversations ADD CONSTRAINT conversations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) NOT VALID;

ALTER TABLE jobs VALIDATE CONSTRAINT jobs_user_id_fkey;
ALTER TABLE podcasts VALIDATE CONSTRAINT podcasts_user_id_fkey;
ALTER TABLE episodes VALIDATE CONSTRAINT episodes_user_id_fkey;
ALTER TABLE episode_sources VALIDATE CONSTRAINT episode_sources_user_id_fkey;
ALTER TABLE episode_identity_keys VALIDATE CONSTRAINT episode_identity_keys_user_id_fkey;
ALTER TABLE imports VALIDATE CONSTRAINT imports_user_id_fkey;
ALTER TABLE notes VALIDATE CONSTRAINT notes_user_id_fkey;
ALTER TABLE transcription_runs VALIDATE CONSTRAINT transcription_runs_user_id_fkey;
ALTER TABLE transcript_versions VALIDATE CONSTRAINT transcript_versions_user_id_fkey;
ALTER TABLE search_documents VALIDATE CONSTRAINT search_documents_user_id_fkey;
ALTER TABLE ai_artifacts VALIDATE CONSTRAINT ai_artifacts_user_id_fkey;
ALTER TABLE conversations VALIDATE CONSTRAINT conversations_user_id_fkey;

COMMIT;
