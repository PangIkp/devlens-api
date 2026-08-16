ALTER TABLE deployments
    ADD COLUMN IF NOT EXISTS github_deployment_id BIGINT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_deployments_github_deployment_id
    ON deployments (github_deployment_id)
    WHERE github_deployment_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS workflow_events (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    github_workflow_run_id BIGINT NOT NULL UNIQUE,
    workflow_name VARCHAR(255) NOT NULL,
    status VARCHAR(64),
    conclusion VARCHAR(64),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_workflow_events_repository_started_at
    ON workflow_events (repository_id, started_at);
