-- name: CreateOrganization :one
INSERT INTO organizations (
    id,
    github_id,
    slug,
    name
) VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING id, github_id, slug, name, created_at, updated_at;

-- name: GetOrganizationByID :one
SELECT id, github_id, slug, name, created_at, updated_at
FROM organizations
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ListOrganizations :many
SELECT id, github_id, slug, name, created_at, updated_at
FROM organizations
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1
OFFSET $2;

-- name: CountOrganizations :one
SELECT COUNT(*)::bigint
FROM organizations
WHERE deleted_at IS NULL;
