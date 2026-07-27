-- name: UpsertPullRequest :one
INSERT INTO pull_requests (
    id,
    repository_id,
    github_pr_id,
    number,
    title,
    author,
    state,
    created_at,
    merged_at,
    closed_at,
    additions,
    deletions,
    files_changed
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13
)
ON CONFLICT (github_pr_id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    number = EXCLUDED.number,
    title = EXCLUDED.title,
    author = EXCLUDED.author,
    state = EXCLUDED.state,
    created_at = EXCLUDED.created_at,
    merged_at = EXCLUDED.merged_at,
    closed_at = EXCLUDED.closed_at,
    additions = EXCLUDED.additions,
    deletions = EXCLUDED.deletions,
    files_changed = EXCLUDED.files_changed
RETURNING id;

-- name: UpsertPullRequestReview :exec
INSERT INTO pull_request_reviews (
    id,
    pull_request_id,
    github_review_id,
    reviewer,
    review_requested_at,
    first_review_at,
    review_submitted_at,
    state
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8
)
ON CONFLICT (github_review_id) DO UPDATE SET
    pull_request_id = EXCLUDED.pull_request_id,
    reviewer = EXCLUDED.reviewer,
    review_requested_at = EXCLUDED.review_requested_at,
    first_review_at = EXCLUDED.first_review_at,
    review_submitted_at = EXCLUDED.review_submitted_at,
    state = EXCLUDED.state;
