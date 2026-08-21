BEGIN;

ALTER TABLE imports DROP CONSTRAINT imports_job_id_fkey;
ALTER TABLE imports
    ADD CONSTRAINT imports_job_id_fkey
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE SET NULL;

COMMIT;
