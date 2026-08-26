BEGIN;

ALTER TABLE episode_sources
    DROP CONSTRAINT episode_sources_source_type_check,
    ADD CONSTRAINT episode_sources_source_type_check
        CHECK (source_type IN ('apple_podcasts', 'rss', 'direct_audio', 'snapany')),
    ADD COLUMN download_headers JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(download_headers) = 'object');

COMMIT;
