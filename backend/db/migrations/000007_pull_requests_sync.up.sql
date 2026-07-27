CREATE TABLE pull_requests (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    github_pr_id BIGINT NOT NULL,
    number INTEGER NOT NULL,
    title VARCHAR(512) NOT NULL,
    author VARCHAR(255) NOT NULL,
    state VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    merged_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    additions INTEGER NOT NULL DEFAULT 0,
    deletions INTEGER NOT NULL DEFAULT 0,
    files_changed INTEGER NOT NULL DEFAULT 0,
    UNIQUE (github_pr_id),
    UNIQUE (repository_id, number)
);

CREATE INDEX idx_pull_requests_repository_id
    ON pull_requests (repository_id);

CREATE TABLE pull_request_reviews (
    id UUID PRIMARY KEY,
    pull_request_id UUID NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    github_review_id BIGINT NOT NULL,
    reviewer VARCHAR(255) NOT NULL,
    review_requested_at TIMESTAMPTZ,
    first_review_at TIMESTAMPTZ,
    review_submitted_at TIMESTAMPTZ,
    state VARCHAR(64) NOT NULL,
    UNIQUE (github_review_id)
);

CREATE INDEX idx_pull_request_reviews_pull_request_id
    ON pull_request_reviews (pull_request_id);
