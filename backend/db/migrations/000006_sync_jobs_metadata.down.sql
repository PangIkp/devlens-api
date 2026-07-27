ALTER TABLE sync_jobs
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS triggered_by;

ALTER TABLE sync_jobs
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status DROP NOT NULL;
