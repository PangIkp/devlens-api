CREATE INDEX IF NOT EXISTS idx_organizations_deleted_at
    ON organizations (deleted_at)
    WHERE deleted_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_github_installations_disconnected_at
    ON github_installations (disconnected_at)
    WHERE disconnected_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_payload_retention
    ON webhook_deliveries (payload_retention_until)
    WHERE payload IS NOT NULL;
