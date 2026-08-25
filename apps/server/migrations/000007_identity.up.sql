BEGIN;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
