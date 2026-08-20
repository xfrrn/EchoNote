BEGIN;

CREATE TABLE podcasts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL CHECK (btrim(title) <> ''),
    author TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    feed_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX podcasts_user_feed_url_idx
    ON podcasts (user_id, feed_url)
    WHERE feed_url IS NOT NULL;

CREATE TABLE episodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    podcast_id UUID REFERENCES podcasts(id) ON DELETE SET NULL,
    title TEXT NOT NULL CHECK (btrim(title) <> ''),
    description TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    cover_url TEXT NOT NULL DEFAULT '',
    resolve_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (resolve_status IN ('pending', 'completed', 'failed')),
    transcription_status TEXT NOT NULL DEFAULT 'waiting'
        CHECK (transcription_status IN ('waiting', 'queued', 'running', 'completed', 'failed')),
    ai_status TEXT NOT NULL DEFAULT 'waiting'
        CHECK (ai_status IN ('waiting', 'queued', 'running', 'completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX episodes_user_created_idx ON episodes (user_id, created_at DESC, id);
CREATE INDEX episodes_podcast_idx ON episodes (podcast_id, published_at DESC, id);

CREATE TABLE episode_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL
        CHECK (source_type IN ('apple_podcasts', 'rss', 'direct_audio')),
    external_id TEXT,
    source_url TEXT NOT NULL CHECK (btrim(source_url) <> ''),
    canonical_url TEXT NOT NULL CHECK (btrim(canonical_url) <> ''),
    audio_url TEXT NOT NULL CHECK (btrim(audio_url) <> ''),
    rss_guid TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, episode_id, source_type, source_url)
);

CREATE INDEX episode_sources_episode_idx ON episode_sources (episode_id, created_at, id);

CREATE TABLE episode_identity_keys (
    user_id UUID NOT NULL,
    identity_key TEXT NOT NULL CHECK (btrim(identity_key) <> ''),
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, identity_key)
);

CREATE INDEX episode_identity_keys_episode_idx ON episode_identity_keys (episode_id);

CREATE TABLE imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    submitted_url TEXT NOT NULL CHECK (btrim(submitted_url) <> ''),
    job_id UUID UNIQUE REFERENCES jobs(id) ON DELETE RESTRICT,
    episode_id UUID REFERENCES episodes(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX imports_user_created_idx ON imports (user_id, created_at DESC, id);

COMMIT;
