\set ON_ERROR_STOP on

-- Run as the target database owner both before the first migration and after grants change:
-- psql -d echonote -v database_name=echonote -v migration_role=echonote_migrate \
--   -v api_role=echonote_api -v worker_role=echonote_worker \
--   -v maintenance_role=echonote_maintenance -f runtime-grants.sql

REVOKE CONNECT ON DATABASE :"database_name" FROM PUBLIC;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

GRANT CONNECT ON DATABASE :"database_name" TO :"migration_role", :"api_role", :"worker_role", :"maintenance_role";
GRANT USAGE, CREATE ON SCHEMA public TO :"migration_role";
GRANT USAGE ON SCHEMA public TO :"api_role", :"worker_role", :"maintenance_role";

REVOKE CREATE ON DATABASE :"database_name" FROM :"api_role", :"worker_role", :"maintenance_role";
REVOKE CREATE ON SCHEMA public FROM :"api_role", :"worker_role", :"maintenance_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO :"api_role", :"worker_role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO :"api_role", :"worker_role";

ALTER DEFAULT PRIVILEGES FOR ROLE :"migration_role" IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"api_role", :"worker_role";
ALTER DEFAULT PRIVILEGES FOR ROLE :"migration_role" IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO :"api_role", :"worker_role";
