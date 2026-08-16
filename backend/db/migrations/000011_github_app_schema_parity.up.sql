ALTER TABLE github_installations
    ADD COLUMN IF NOT EXISTS disconnected_at TIMESTAMPTZ;

ALTER TABLE github_installation_repositories
    ADD COLUMN IF NOT EXISTS last_discovered_at TIMESTAMPTZ;

UPDATE github_installation_repositories
SET last_discovered_at = COALESCE(last_discovered_at, created_at, NOW())
WHERE last_discovered_at IS NULL;

ALTER TABLE repositories
    ADD COLUMN IF NOT EXISTS github_installation_repository_id UUID REFERENCES github_installation_repositories(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS initial_sync_status VARCHAR(64),
    ADD COLUMN IF NOT EXISTS initial_sync_completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS sync_error_message TEXT;

ALTER TABLE sync_jobs
    ADD COLUMN IF NOT EXISTS job_type VARCHAR(64) NOT NULL DEFAULT 'repository_sync',
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255),
    ADD COLUMN IF NOT EXISTS error_code VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_jobs_idempotency_key
    ON sync_jobs (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sync_jobs_repository_status
    ON sync_jobs (repository_id, status);

CREATE TABLE IF NOT EXISTS sync_checkpoints (
    id UUID PRIMARY KEY,
    sync_job_id UUID NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    resource_type VARCHAR(64) NOT NULL,
    checkpoint_key VARCHAR(255) NOT NULL,
    checkpoint_value TEXT,
    status VARCHAR(64) NOT NULL,
    last_processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_checkpoints_job_resource_key
    ON sync_checkpoints (sync_job_id, resource_type, checkpoint_key);

CREATE INDEX IF NOT EXISTS idx_sync_checkpoints_repository_resource
    ON sync_checkpoints (repository_id, resource_type);

ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS github_installation_id UUID REFERENCES github_installations(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_installation_id
    ON webhook_deliveries (github_installation_id);
