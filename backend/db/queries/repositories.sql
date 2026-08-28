-- name: CreateRepository :one
INSERT INTO repositories (
    id,
    organization_id,
    github_id,
    name,
    full_name,
    default_branch,
    is_active,
    archived_at
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
RETURNING id, organization_id, github_id, name, full_name, default_branch, is_active, archived_at, last_synced_at, created_at, updated_at;

-- name: GetRepositoryByID :one
SELECT id, organization_id, github_id, name, full_name, default_branch, is_active, archived_at, last_synced_at, created_at, updated_at
FROM repositories
WHERE id = $1;

-- name: GetRepositoryByGithubID :one
SELECT id, organization_id, github_id, name, full_name, default_branch, is_active, archived_at, last_synced_at, created_at, updated_at
FROM repositories
WHERE github_id = $1;

-- name: ListRepositories :many
SELECT id, organization_id, github_id, name, full_name, default_branch, is_active, archived_at, last_synced_at, created_at, updated_at
FROM repositories
WHERE organization_id = $1
  AND (
    $2 = ''
    OR ($2 = 'active' AND archived_at IS NULL AND is_active = TRUE)
    OR ($2 = 'inactive' AND archived_at IS NULL AND is_active = FALSE)
    OR ($2 = 'archived' AND archived_at IS NOT NULL)
  )
  AND (
    $3 = ''
    OR name ILIKE '%' || $3 || '%'
    OR full_name ILIKE '%' || $3 || '%'
  )
ORDER BY
  CASE WHEN $4 = 'name' AND $5 = 'asc' THEN name END ASC,
  CASE WHEN $4 = 'name' AND $5 = 'desc' THEN name END DESC,
  CASE WHEN $4 = 'fullName' AND $5 = 'asc' THEN full_name END ASC,
  CASE WHEN $4 = 'fullName' AND $5 = 'desc' THEN full_name END DESC,
  CASE WHEN $4 = 'createdAt' AND $5 = 'asc' THEN created_at END ASC,
  CASE WHEN $4 = 'createdAt' AND $5 = 'desc' THEN created_at END DESC,
  id ASC
LIMIT $6
OFFSET $7;

-- name: CountRepositories :one
SELECT COUNT(*)::bigint
FROM repositories
WHERE organization_id = $1
  AND (
    $2 = ''
    OR ($2 = 'active' AND archived_at IS NULL AND is_active = TRUE)
    OR ($2 = 'inactive' AND archived_at IS NULL AND is_active = FALSE)
    OR ($2 = 'archived' AND archived_at IS NOT NULL)
  )
  AND (
    $3 = ''
    OR name ILIKE '%' || $3 || '%'
    OR full_name ILIKE '%' || $3 || '%'
  );

-- name: UpdateRepository :one
UPDATE repositories
SET name = $2,
    full_name = $3,
    default_branch = $4,
    is_active = $5,
    archived_at = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING id, organization_id, github_id, name, full_name, default_branch, is_active, archived_at, last_synced_at, created_at, updated_at;

-- name: SyncRepositoryMetadata :exec
UPDATE repositories
SET name = $2,
    full_name = $3,
    default_branch = $4,
  archived_at = $5,
  updated_at = $6
WHERE id = $1;

-- name: UpdateRepositoryLastSyncedAt :exec
UPDATE repositories
SET last_synced_at = $2,
    updated_at = $2
WHERE id = $1;

-- name: RepositoryOrganizationExists :one
SELECT EXISTS (
    SELECT 1
    FROM organizations
    WHERE id = $1
      AND deleted_at IS NULL
) AS exists;
