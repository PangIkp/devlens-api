DROP INDEX IF EXISTS idx_webhook_deliveries_retry_schedule;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS retry_count;
