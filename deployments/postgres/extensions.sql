\set ON_ERROR_STOP on

-- Run once as the managed PostgreSQL administrator after installing the pgvector package
-- that matches the server major version.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS vector;
