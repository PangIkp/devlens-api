ALTER TABLE pull_requests
    ADD COLUMN IF NOT EXISTS is_draft BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE deployments (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    environment VARCHAR(255) NOT NULL,
    status VARCHAR(64) NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_deployments_repository_id_deployed_at
    ON deployments (repository_id, deployed_at);

CREATE TABLE file_changes (
    id UUID PRIMARY KEY,
    pull_request_id UUID NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    file_path VARCHAR(1024) NOT NULL,
    additions INTEGER NOT NULL DEFAULT 0,
    deletions INTEGER NOT NULL DEFAULT 0,
    commit_count INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_file_changes_pull_request_id
    ON file_changes (pull_request_id);
