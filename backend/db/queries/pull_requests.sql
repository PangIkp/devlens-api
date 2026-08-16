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
    files_changed,
    is_draft
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
    $13,
    $14
)
ON CONFLICT (github_pr_id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    number = EXCLUDED.number,
    title = EXCLUDED.title,
    author = EXCLUDED.author,
    state = CASE
        WHEN pull_requests.merged_at IS NOT NULL AND EXCLUDED.merged_at IS NULL THEN pull_requests.state
        WHEN pull_requests.closed_at IS NOT NULL AND EXCLUDED.closed_at IS NULL AND EXCLUDED.state = 'open' THEN pull_requests.state
        ELSE EXCLUDED.state
    END,
    created_at = LEAST(pull_requests.created_at, EXCLUDED.created_at),
    merged_at = COALESCE(GREATEST(pull_requests.merged_at, EXCLUDED.merged_at), pull_requests.merged_at, EXCLUDED.merged_at),
    closed_at = COALESCE(GREATEST(pull_requests.closed_at, EXCLUDED.closed_at), pull_requests.closed_at, EXCLUDED.closed_at),
    additions = EXCLUDED.additions,
    deletions = EXCLUDED.deletions,
    files_changed = EXCLUDED.files_changed,
    is_draft = EXCLUDED.is_draft
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
    review_requested_at = COALESCE(LEAST(pull_request_reviews.review_requested_at, EXCLUDED.review_requested_at), pull_request_reviews.review_requested_at, EXCLUDED.review_requested_at),
    first_review_at = COALESCE(LEAST(pull_request_reviews.first_review_at, EXCLUDED.first_review_at), pull_request_reviews.first_review_at, EXCLUDED.first_review_at),
    review_submitted_at = COALESCE(GREATEST(pull_request_reviews.review_submitted_at, EXCLUDED.review_submitted_at), pull_request_reviews.review_submitted_at, EXCLUDED.review_submitted_at),
    state = CASE
        WHEN EXCLUDED.review_submitted_at IS NOT NULL
             AND (pull_request_reviews.review_submitted_at IS NULL OR EXCLUDED.review_submitted_at >= pull_request_reviews.review_submitted_at)
        THEN EXCLUDED.state
        ELSE pull_request_reviews.state
    END;
