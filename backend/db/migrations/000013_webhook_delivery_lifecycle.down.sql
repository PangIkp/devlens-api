DROP INDEX IF EXISTS idx_webhook_deliveries_processing_status;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS processed_at,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS payload_retention_until,
    DROP COLUMN IF EXISTS processing_status;
