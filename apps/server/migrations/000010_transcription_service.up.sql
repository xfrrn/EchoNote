BEGIN;

ALTER TABLE users
    ADD COLUMN auth_issuer TEXT,
    ADD COLUMN auth_subject TEXT,
    ADD COLUMN email TEXT,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD CONSTRAINT users_auth_identity_check CHECK (
        (auth_issuer IS NULL AND auth_subject IS NULL)
        OR
        (btrim(auth_issuer) <> '' AND btrim(auth_subject) <> '')
    );

CREATE UNIQUE INDEX users_auth_identity_idx
    ON users (auth_issuer, auth_subject)
    WHERE auth_issuer IS NOT NULL AND auth_subject IS NOT NULL;

DELETE FROM jobs
WHERE type IN (
    'build_keyword_index',
    'generate_embeddings',
    'generate_ai_artifact',
    'generate_conversation_reply',
    'cancel_asr'
);

DROP TABLE IF EXISTS message_citations;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS ai_artifacts;
DROP TABLE IF EXISTS search_chunks;
DROP TABLE IF EXISTS search_documents;
DROP TABLE IF EXISTS notes;

ALTER TABLE episodes
    DROP COLUMN IF EXISTS ai_status,
    DROP COLUMN IF EXISTS podcast_id;

DROP TABLE IF EXISTS podcasts;

COMMIT;
