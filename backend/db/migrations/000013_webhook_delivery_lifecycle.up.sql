ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS processing_status VARCHAR(64) NOT NULL DEFAULT 'received',
    ADD COLUMN IF NOT EXISTS payload_retention_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS error_message TEXT,
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;

UPDATE webhook_deliveries
SET processing_status = CASE
        WHEN processed = TRUE THEN 'processed'
        WHEN sync_job_id IS NOT NULL THEN 'enqueued'
        ELSE 'ignored'
    END,
    processed_at = CASE
        WHEN processed = TRUE THEN COALESCE(processed_at, updated_at, received_at)
        ELSE processed_at
    END,
    payload_retention_until = COALESCE(payload_retention_until, received_at + INTERVAL '30 days')
WHERE processing_status IS NULL
   OR payload_retention_until IS NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_processing_status
    ON webhook_deliveries (processing_status, received_at);
