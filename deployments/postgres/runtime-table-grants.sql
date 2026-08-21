\set ON_ERROR_STOP on

-- Run after migrations as the target database owner.
REVOKE ALL PRIVILEGES ON TABLE users, sessions FROM :"worker_role";
REVOKE INSERT, UPDATE, DELETE ON TABLE notes FROM :"worker_role";
GRANT SELECT ON TABLE notes TO :"worker_role";
