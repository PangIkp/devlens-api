ALTER TABLE github_installations
    ADD COLUMN IF NOT EXISTS account_login VARCHAR(255),
    ADD COLUMN IF NOT EXISTS account_type VARCHAR(64),
    ADD COLUMN IF NOT EXISTS target_type VARCHAR(64),
    ADD COLUMN IF NOT EXISTS status VARCHAR(64) NOT NULL DEFAULT 'installation_required',
    ADD COLUMN IF NOT EXISTS permissions_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS installed_by_github_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_installations_organization_unique
    ON github_installations (organization_id);

CREATE TABLE IF NOT EXISTS github_installation_repositories (
    id UUID PRIMARY KEY,
    github_installation_id UUID NOT NULL REFERENCES github_installations(id) ON DELETE CASCADE,
    github_repository_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    owner_login VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    private BOOLEAN NOT NULL DEFAULT FALSE,
    default_branch VARCHAR(255),
    installation_status VARCHAR(64) NOT NULL,
    selection_status VARCHAR(64) NOT NULL DEFAULT 'not_selected',
    permissions_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    linked_repository_id UUID REFERENCES repositories(id) ON DELETE SET NULL,
    last_synced_at TIMESTAMPTZ,
    sync_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_installation_repositories_installation_repo
    ON github_installation_repositories (github_installation_id, github_repository_id);

CREATE INDEX IF NOT EXISTS idx_github_installation_repositories_linked_repository
    ON github_installation_repositories (linked_repository_id);
