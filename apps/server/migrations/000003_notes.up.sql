BEGIN;

CREATE TABLE notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    client_note_id UUID NOT NULL,
    content TEXT NOT NULL CHECK (btrim(content) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (user_id, client_note_id)
);

CREATE INDEX notes_episode_created_idx
    ON notes (episode_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

COMMIT;
