#!/usr/bin/env bash
set -euo pipefail

: "${PGSERVICE:?set PGSERVICE to an isolated restore service name}"
: "${RESTORE_EXPECTED_DATABASE:?set RESTORE_EXPECTED_DATABASE}"
: "${EXPECTED_MIGRATION_VERSION:?set EXPECTED_MIGRATION_VERSION}"

[[ "$PGSERVICE" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "invalid PGSERVICE" >&2; exit 2; }
[[ "$RESTORE_EXPECTED_DATABASE" =~ ^echonote_restore_[a-z0-9_]+$ ]] || { echo "restore database must start with echonote_restore_" >&2; exit 2; }
[[ "$EXPECTED_MIGRATION_VERSION" =~ ^[0-9]+$ ]] || { echo "EXPECTED_MIGRATION_VERSION must be numeric" >&2; exit 2; }

psql_args=(-X --no-psqlrc --set ON_ERROR_STOP=1 --dbname "service=$PGSERVICE")
actual_database="$(psql "${psql_args[@]}" -Atc 'SELECT current_database()')"
[[ "$actual_database" == "$RESTORE_EXPECTED_DATABASE" ]] || {
  echo "connected to $actual_database, expected $RESTORE_EXPECTED_DATABASE" >&2
  exit 1
}

IFS='|' read -r migration_version migration_dirty < <(
  psql "${psql_args[@]}" -Aqt -F '|' -c 'SELECT version, dirty FROM schema_migrations'
)
[[ "$migration_version" == "$EXPECTED_MIGRATION_VERSION" && "$migration_dirty" == "f" ]] || {
  echo "migration version=$migration_version dirty=$migration_dirty" >&2
  exit 1
}

extensions="$(psql "${psql_args[@]}" -Atc "SELECT string_agg(extname, ',' ORDER BY extname) FROM pg_extension WHERE extname IN ('pg_trgm','vector')")"
[[ "$extensions" == "pg_trgm,vector" ]] || { echo "required extensions missing: $extensions" >&2; exit 1; }

invalid_foreign_keys="$(psql "${psql_args[@]}" -Atc "SELECT count(*) FROM pg_constraint WHERE contype='f' AND NOT convalidated")"
[[ "$invalid_foreign_keys" == "0" ]] || { echo "$invalid_foreign_keys unvalidated foreign keys" >&2; exit 1; }

orphaned="$(psql "${psql_args[@]}" -Atc '
SELECT
  (SELECT count(*) FROM notes n LEFT JOIN episodes e ON e.id=n.episode_id WHERE e.id IS NULL) +
  (SELECT count(*) FROM transcript_versions v LEFT JOIN episodes e ON e.id=v.episode_id WHERE e.id IS NULL) +
  (SELECT count(*) FROM transcript_segments s LEFT JOIN transcript_speakers p ON p.id=s.speaker_id WHERE p.id IS NULL) +
  (SELECT count(*) FROM search_chunks c LEFT JOIN search_documents d ON d.id=c.search_document_id WHERE d.id IS NULL) +
  (SELECT count(*) FROM ai_artifacts a LEFT JOIN transcript_versions v ON v.id=a.transcript_version_id WHERE v.id IS NULL) +
  (SELECT count(*) FROM message_citations c LEFT JOIN messages m ON m.id=c.message_id WHERE m.id IS NULL)')"
[[ "$orphaned" == "0" ]] || { echo "$orphaned orphaned business rows" >&2; exit 1; }

psql "${psql_args[@]}" --pset pager=off -c "
SELECT 'episodes' AS relation, count(*) FROM episodes
UNION ALL SELECT 'notes', count(*) FROM notes
UNION ALL SELECT 'transcript_versions', count(*) FROM transcript_versions
UNION ALL SELECT 'transcript_speakers', count(*) FROM transcript_speakers
UNION ALL SELECT 'transcript_segments', count(*) FROM transcript_segments
UNION ALL SELECT 'search_documents', count(*) FROM search_documents
UNION ALL SELECT 'ai_artifacts', count(*) FROM ai_artifacts
UNION ALL SELECT 'message_citations', count(*) FROM message_citations
ORDER BY relation"
psql "${psql_args[@]}" --pset pager=off -c "
SELECT type, status, count(*)
FROM jobs
WHERE status IN ('queued','running','failed')
GROUP BY type, status
ORDER BY type, status"

echo "restore verification passed: database=$actual_database migration=$migration_version"
