-- name: SyncJobRepositoryExists :one
SELECT EXISTS (
    SELECT 1
    FROM repositories
    WHERE id = $1
);

-- name: GetSyncJobRepositoryTarget :one
SELECT
    r.id,
    r.full_name,
    r.last_synced_at,
    r.github_installation_repository_id,
    gi.installation_id,
    gi.status AS installation_status
FROM repositories r
LEFT JOIN github_installation_repositories gir ON gir.id = r.github_installation_repository_id
LEFT JOIN github_installations gi ON gi.id = gir.github_installation_id
WHERE r.id = $1;

-- name: HasActiveSyncJob :one
SELECT EXISTS (
    SELECT 1
    FROM sync_jobs
    WHERE repository_id = $1
      AND status IN ('pending', 'running')
);

-- name: CreateSyncJob :one
INSERT INTO sync_jobs (
    id,
    repository_id,
    status,
    progress,
    triggered_by,
    error_message,
    started_at,
    finished_at,
    created_at,
    updated_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    NOW(),
    NULL
)
RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at;

-- name: GetSyncJobByID :one
SELECT id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at
FROM sync_jobs
WHERE id = $1;

-- name: ListPendingSyncJobs :many
SELECT id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at
FROM sync_jobs
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT $1;

-- name: ListSyncJobsByRepository :many
SELECT id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at
FROM sync_jobs
WHERE repository_id = $1
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY
  CASE
    WHEN sqlc.arg(sort_order)::text = 'asc' THEN created_at
  END ASC,
  CASE
    WHEN sqlc.arg(sort_order)::text <> 'asc' THEN created_at
  END DESC,
  id DESC
LIMIT $2
OFFSET $3;

-- name: CountSyncJobsByRepository :one
SELECT COUNT(*)
FROM sync_jobs
WHERE repository_id = $1
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: UpdateSyncJobRunning :one
UPDATE sync_jobs
SET status = 'running',
    progress = $2,
    started_at = COALESCE(started_at, $3),
    finished_at = NULL,
    error_message = NULL,
    updated_at = $3
WHERE id = $1
RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at;

-- name: UpdateSyncJobProgress :one
UPDATE sync_jobs
SET progress = $2,
    updated_at = $3
WHERE id = $1
RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at;

-- name: UpdateSyncJobFailed :one
UPDATE sync_jobs
SET status = 'failed',
    error_message = $2,
    finished_at = $3,
    updated_at = $3
WHERE id = $1
RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at;

-- name: UpdateSyncJobCompleted :one
UPDATE sync_jobs
SET status = 'completed',
    progress = 100,
    error_message = NULL,
    finished_at = $2,
    updated_at = $2
WHERE id = $1
RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at;
