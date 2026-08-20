BEGIN;

DELETE FROM jobs
WHERE type IN ('build_keyword_index', 'generate_embeddings');

DROP TABLE IF EXISTS search_chunks;
DROP TABLE IF EXISTS search_documents;

COMMIT;
