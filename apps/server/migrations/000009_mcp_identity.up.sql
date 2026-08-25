BEGIN;

DROP TABLE IF EXISTS sessions;

ALTER TABLE users
    DROP COLUMN IF EXISTS username,
    DROP COLUMN IF EXISTS username_normalized,
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS updated_at;

COMMIT;
