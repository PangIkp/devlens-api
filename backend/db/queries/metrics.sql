-- name: AggregatePRSizeByDay :many
WITH review_counts AS (
    SELECT pull_request_id, COUNT(*) AS review_count
    FROM pull_request_reviews
    GROUP BY pull_request_id
)
SELECT DATE(pr.created_at AT TIME ZONE 'UTC') AS metric_date,
       COUNT(*)::bigint AS pr_count,
       COUNT(*) FILTER (WHERE COALESCE(rc.review_count, 0) > 0)::bigint AS reviewed_pr_count,
       COALESCE(AVG(pr.files_changed)::double precision, 0)::double precision AS average_files_changed,
       COALESCE(AVG(pr.additions)::double precision, 0)::double precision AS average_additions,
       COALESCE(AVG(pr.deletions)::double precision, 0)::double precision AS average_deletions
FROM pull_requests pr
LEFT JOIN review_counts rc ON rc.pull_request_id = pr.id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
  AND pr.is_draft = FALSE
GROUP BY DATE(pr.created_at AT TIME ZONE 'UTC')
ORDER BY metric_date;

-- name: AggregatePRCycleByDay :many
SELECT DATE(pr.merged_at AT TIME ZONE 'UTC') AS metric_date,
       COUNT(*)::bigint AS merged_pr_count,
       COALESCE(AVG(EXTRACT(EPOCH FROM (pr.merged_at - pr.created_at)) / 60.0), 0)::double precision AS pr_cycle_time_minutes
FROM pull_requests pr
WHERE pr.repository_id = $1
  AND pr.merged_at IS NOT NULL
  AND pr.merged_at >= $2
  AND pr.merged_at < $3
  AND pr.is_draft = FALSE
GROUP BY DATE(pr.merged_at AT TIME ZONE 'UTC')
ORDER BY metric_date;

-- name: AggregateReviewMetricsByDay :many
SELECT DATE(pr.created_at AT TIME ZONE 'UTC') AS metric_date,
       COUNT(*) FILTER (
           WHERE rev.review_requested_at IS NOT NULL
             AND rev.first_review_at IS NOT NULL
       )::bigint AS review_wait_sample_count,
       COALESCE(AVG(EXTRACT(EPOCH FROM (rev.first_review_at - rev.review_requested_at)) / 60.0) FILTER (
           WHERE rev.review_requested_at IS NOT NULL
             AND rev.first_review_at IS NOT NULL
       ), 0)::double precision AS average_review_wait_minutes,
       COUNT(*) FILTER (
           WHERE rev.review_requested_at IS NOT NULL
             AND rev.review_submitted_at IS NOT NULL
       )::bigint AS review_time_sample_count,
       COALESCE(AVG(EXTRACT(EPOCH FROM (rev.review_submitted_at - rev.review_requested_at)) / 60.0) FILTER (
           WHERE rev.review_requested_at IS NOT NULL
             AND rev.review_submitted_at IS NOT NULL
       ), 0)::double precision AS average_review_minutes
FROM pull_requests pr
LEFT JOIN pull_request_reviews rev ON rev.pull_request_id = pr.id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
  AND pr.is_draft = FALSE
GROUP BY DATE(pr.created_at AT TIME ZONE 'UTC')
ORDER BY metric_date;

-- name: AggregateDeploymentsByDay :many
SELECT DATE(deployed_at AT TIME ZONE 'UTC') AS metric_date,
       COUNT(*) FILTER (WHERE status = 'success')::bigint AS successful_deployment_count,
       COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed_deployment_count
FROM deployments
WHERE repository_id = $1
  AND deployed_at >= $2
  AND deployed_at < $3
GROUP BY DATE(deployed_at AT TIME ZONE 'UTC')
ORDER BY metric_date;

-- name: ListPullRequestsForAnalytics :many
SELECT id, repository_id, github_pr_id, number, title, author, state, created_at, merged_at, closed_at, additions, deletions, files_changed, is_draft
FROM pull_requests
WHERE repository_id = $1
  AND created_at < $3
  AND (merged_at IS NULL OR merged_at >= $2 OR created_at >= $2)
ORDER BY created_at ASC;

-- name: ListPullRequestReviewsForAnalytics :many
SELECT prr.id, prr.pull_request_id, prr.github_review_id, prr.reviewer, prr.review_requested_at, prr.first_review_at, prr.review_submitted_at, prr.state
FROM pull_request_reviews prr
INNER JOIN pull_requests pr ON pr.id = prr.pull_request_id
WHERE pr.repository_id = $1
  AND pr.created_at < $3
  AND (pr.merged_at IS NULL OR pr.merged_at >= $2 OR pr.created_at >= $2)
ORDER BY prr.id ASC;

-- name: ListDeploymentsForAnalytics :many
SELECT id, repository_id, environment, status, deployed_at
FROM deployments
WHERE repository_id = $1
  AND deployed_at >= $2
  AND deployed_at < $3
ORDER BY deployed_at ASC;

-- name: ListFileChangesForAnalytics :many
SELECT fc.id, fc.pull_request_id, fc.file_path, fc.additions, fc.deletions, fc.commit_count
FROM file_changes fc
INNER JOIN pull_requests pr ON pr.id = fc.pull_request_id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
ORDER BY fc.id ASC;
