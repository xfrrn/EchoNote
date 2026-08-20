BEGIN;

CREATE TABLE transcription_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    profile TEXT NOT NULL CHECK (profile IN ('economy', 'quality')),
    provider TEXT NOT NULL CHECK (btrim(provider) <> ''),
    model TEXT NOT NULL CHECK (btrim(model) <> ''),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'downloading', 'preparing', 'transcribing', 'aligning', 'merging', 'completed', 'failed', 'canceled')),
    stage TEXT NOT NULL DEFAULT 'queued' CHECK (btrim(stage) <> ''),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_object_key TEXT,
    source_audio_hash TEXT CHECK (source_audio_hash IS NULL OR source_audio_hash ~ '^[0-9a-f]{64}$'),
    prepared_object_key TEXT,
    prepared_audio_hash TEXT CHECK (prepared_audio_hash IS NULL OR prepared_audio_hash ~ '^[0-9a-f]{64}$'),
    duration_ms BIGINT CHECK (duration_ms IS NULL OR duration_ms > 0),
    total_chunks INTEGER NOT NULL DEFAULT 0 CHECK (total_chunks >= 0),
    completed_chunks INTEGER NOT NULL DEFAULT 0 CHECK (completed_chunks >= 0 AND completed_chunks <= total_chunks),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_code TEXT,
    error_message TEXT,
    audio_cleaned_at TIMESTAMPTZ,
    chunks_cleaned_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status IN ('completed', 'failed', 'canceled')) = (completed_at IS NOT NULL))
);

CREATE UNIQUE INDEX transcription_runs_episode_active_idx
    ON transcription_runs (episode_id)
    WHERE status NOT IN ('completed', 'failed', 'canceled');
CREATE INDEX transcription_runs_user_created_idx
    ON transcription_runs (user_id, created_at DESC, id DESC);

CREATE TABLE transcription_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transcription_run_id UUID NOT NULL REFERENCES transcription_runs(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    core_start_ms BIGINT NOT NULL CHECK (core_start_ms >= 0),
    core_end_ms BIGINT NOT NULL CHECK (core_end_ms > core_start_ms),
    render_start_ms BIGINT NOT NULL CHECK (render_start_ms >= 0 AND render_start_ms <= core_start_ms),
    render_end_ms BIGINT NOT NULL CHECK (render_end_ms >= core_end_ms),
    status TEXT NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'rendering', 'ready', 'submitting', 'submitted', 'running', 'completed', 'failed', 'canceled')),
    object_key TEXT,
    audio_hash TEXT CHECK (audio_hash IS NULL OR audio_hash ~ '^[0-9a-f]{64}$'),
    fingerprint TEXT CHECK (fingerprint IS NULL OR fingerprint ~ '^[0-9a-f]{64}$'),
    external_task_id TEXT,
    result_url TEXT,
    raw_result_object_key TEXT,
    normalized_result JSONB,
    speaker_map JSONB,
    alignment_low_confidence BOOLEAN NOT NULL DEFAULT false,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (transcription_run_id, sequence),
    CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

CREATE INDEX transcription_chunks_run_status_idx
    ON transcription_chunks (transcription_run_id, status, sequence);

CREATE TABLE transcription_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transcription_run_id UUID NOT NULL REFERENCES transcription_runs(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (btrim(event_type) <> ''),
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX transcription_events_run_id_idx
    ON transcription_events (transcription_run_id, id);

CREATE TABLE transcript_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    transcription_run_id UUID NOT NULL UNIQUE REFERENCES transcription_runs(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version > 0),
    is_active BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('completed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (episode_id, version)
);

CREATE UNIQUE INDEX transcript_versions_episode_active_idx
    ON transcript_versions (episode_id)
    WHERE is_active;
CREATE INDEX transcript_versions_user_episode_idx
    ON transcript_versions (user_id, episode_id, version DESC);

CREATE TABLE transcript_speakers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transcript_version_id UUID NOT NULL REFERENCES transcript_versions(id) ON DELETE CASCADE,
    stable_key TEXT NOT NULL CHECK (btrim(stable_key) <> ''),
    display_name TEXT NOT NULL CHECK (btrim(display_name) <> ''),
    role TEXT NOT NULL DEFAULT 'unknown' CHECK (btrim(role) <> ''),
    speaker_profile_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (transcript_version_id, stable_key)
);

CREATE TABLE transcript_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transcript_version_id UUID NOT NULL REFERENCES transcript_versions(id) ON DELETE CASCADE,
    speaker_id UUID NOT NULL REFERENCES transcript_speakers(id) ON DELETE RESTRICT,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    start_ms BIGINT NOT NULL CHECK (start_ms >= 0),
    end_ms BIGINT NOT NULL CHECK (end_ms > start_ms),
    text TEXT NOT NULL CHECK (btrim(text) <> ''),
    words JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_chunk_id UUID NOT NULL REFERENCES transcription_chunks(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (transcript_version_id, sequence)
);

CREATE INDEX transcript_segments_version_time_idx
    ON transcript_segments (transcript_version_id, start_ms, sequence);
CREATE INDEX transcript_segments_speaker_idx
    ON transcript_segments (speaker_id, sequence);

COMMIT;
