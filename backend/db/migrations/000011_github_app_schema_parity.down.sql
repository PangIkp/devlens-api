DROP INDEX IF EXISTS idx_webhook_deliveries_installation_id;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS github_installation_id;

DROP INDEX IF EXISTS idx_sync_checkpoints_repository_resource;
DROP INDEX IF EXISTS idx_sync_checkpoints_job_resource_key;

DROP TABLE IF EXISTS sync_checkpoints;

DROP INDEX IF EXISTS idx_sync_jobs_repository_status;
DROP INDEX IF EXISTS idx_sync_jobs_idempotency_key;

ALTER TABLE sync_jobs
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS job_type;

ALTER TABLE repositories
    DROP COLUMN IF EXISTS sync_error_message,
    DROP COLUMN IF EXISTS initial_sync_completed_at,
    DROP COLUMN IF EXISTS initial_sync_status,
    DROP COLUMN IF EXISTS github_installation_repository_id;

ALTER TABLE github_installation_repositories
    DROP COLUMN IF EXISTS last_discovered_at;

ALTER TABLE github_installations
    DROP COLUMN IF EXISTS disconnected_at;
