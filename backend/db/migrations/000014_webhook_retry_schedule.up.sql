ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retry_schedule
    ON webhook_deliveries (processing_status, next_retry_at, received_at)
    WHERE processing_status = 'failed';
