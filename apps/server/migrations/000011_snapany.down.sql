BEGIN;

ALTER TABLE episode_sources
    DROP COLUMN download_headers;

COMMIT;
