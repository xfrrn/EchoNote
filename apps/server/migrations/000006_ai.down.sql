BEGIN;

DELETE FROM jobs
WHERE type = 'generate_ai_artifact';

DELETE FROM search_documents
WHERE document_type = 'ai_artifact';

DROP TABLE IF EXISTS message_citations;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS ai_artifacts;

COMMIT;
