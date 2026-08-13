CREATE TABLE IF NOT EXISTS commit_events (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    github_commit_sha VARCHAR(255) NOT NULL UNIQUE,
    author VARCHAR(255) NOT NULL,
    author_email VARCHAR(255),
    message TEXT NOT NULL,
    authored_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_commit_events_repository_authored_at
    ON commit_events (repository_id, authored_at);
