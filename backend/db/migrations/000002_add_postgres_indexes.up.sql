CREATE INDEX idx_organization_members_organization_id
    ON organization_members (organization_id);

CREATE INDEX idx_organization_members_user_id
    ON organization_members (user_id);

CREATE INDEX idx_github_installations_organization_id
    ON github_installations (organization_id);

CREATE INDEX idx_repositories_organization_id
    ON repositories (organization_id);

CREATE INDEX idx_sync_jobs_repository_id
    ON sync_jobs (repository_id);

CREATE INDEX idx_webhook_deliveries_repository_id
    ON webhook_deliveries (repository_id);
