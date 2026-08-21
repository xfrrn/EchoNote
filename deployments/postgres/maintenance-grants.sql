\set ON_ERROR_STOP on

-- Run after migrations as the target database owner.
REVOKE ALL PRIVILEGES ON TABLE sessions, jobs, job_events FROM :"maintenance_role";
GRANT DELETE ON TABLE sessions TO :"maintenance_role";
GRANT SELECT (expires_at, revoked_at) ON TABLE sessions TO :"maintenance_role";
GRANT SELECT, DELETE ON TABLE jobs TO :"maintenance_role";
GRANT SELECT ON TABLE job_events TO :"maintenance_role";
GRANT UPDATE (
    status, stage, attempt, run_after, locked_by, locked_at,
    error_code, error_message, updated_at, completed_at
) ON TABLE jobs TO :"maintenance_role";
GRANT INSERT (job_id, event_type, stage, data) ON TABLE job_events TO :"maintenance_role";
