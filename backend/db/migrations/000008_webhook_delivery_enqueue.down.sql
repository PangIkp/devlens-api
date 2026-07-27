ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS sync_job_id,
    DROP COLUMN IF EXISTS payload,
    DROP COLUMN IF EXISTS action;

ALTER TABLE webhook_deliveries
    ALTER COLUMN repository_id SET NOT NULL;
