BEGIN;

ALTER TABLE users
    ADD COLUMN username TEXT,
    ADD COLUMN username_normalized TEXT,
    ADD COLUMN password_hash TEXT,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'placeholder',
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD CONSTRAINT users_status_check CHECK (status IN ('placeholder', 'active', 'disabled')),
    ADD CONSTRAINT users_username_pair_check CHECK ((username IS NULL) = (username_normalized IS NULL)),
    ADD CONSTRAINT users_username_check CHECK (username IS NULL OR btrim(username) <> ''),
    ADD CONSTRAINT users_username_normalized_check CHECK (username_normalized IS NULL OR btrim(username_normalized) <> ''),
    ADD CONSTRAINT users_password_hash_check CHECK (password_hash IS NULL OR btrim(password_hash) <> ''),
    ADD CONSTRAINT users_active_credentials_check CHECK (status <> 'active' OR (username IS NOT NULL AND password_hash IS NOT NULL)),
    ADD CONSTRAINT users_placeholder_credentials_check CHECK (status <> 'placeholder' OR (username IS NULL AND password_hash IS NULL));

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

COMMIT;
