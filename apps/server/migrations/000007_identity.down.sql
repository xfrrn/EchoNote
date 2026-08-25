BEGIN;

ALTER TABLE conversations DROP CONSTRAINT conversations_user_id_fkey;
ALTER TABLE ai_artifacts DROP CONSTRAINT ai_artifacts_user_id_fkey;
ALTER TABLE search_documents DROP CONSTRAINT search_documents_user_id_fkey;
ALTER TABLE transcript_versions DROP CONSTRAINT transcript_versions_user_id_fkey;
ALTER TABLE transcription_runs DROP CONSTRAINT transcription_runs_user_id_fkey;
ALTER TABLE notes DROP CONSTRAINT notes_user_id_fkey;
ALTER TABLE imports DROP CONSTRAINT imports_user_id_fkey;
ALTER TABLE episode_identity_keys DROP CONSTRAINT episode_identity_keys_user_id_fkey;
ALTER TABLE episode_sources DROP CONSTRAINT episode_sources_user_id_fkey;
ALTER TABLE episodes DROP CONSTRAINT episodes_user_id_fkey;
ALTER TABLE podcasts DROP CONSTRAINT podcasts_user_id_fkey;
ALTER TABLE jobs DROP CONSTRAINT jobs_user_id_fkey;

DROP TABLE IF EXISTS sessions;
DROP TABLE users;

COMMIT;
