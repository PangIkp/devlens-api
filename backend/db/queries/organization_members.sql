-- name: ListOrganizationMembers :many
SELECT id, organization_id, user_id, role
FROM organization_members
WHERE organization_id = $1
ORDER BY id ASC
LIMIT 1000;

-- name: GetOrganizationMemberByID :one
SELECT id, organization_id, user_id, role
FROM organization_members
WHERE id = $1;

-- name: CreateOrganizationMember :one
INSERT INTO organization_members (
    id,
    organization_id,
    user_id,
    role
) VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING id, organization_id, user_id, role;

-- name: UpdateOrganizationMemberRole :one
UPDATE organization_members
SET role = $2
WHERE id = $1
RETURNING id, organization_id, user_id, role;

-- name: DeleteOrganizationMember :execrows
DELETE FROM organization_members
WHERE id = $1;

-- name: CountOrganizationOwners :one
SELECT COUNT(*)::bigint
FROM organization_members
WHERE organization_id = $1
  AND role = 'owner';

-- name: LockOrganizationOwnerRows :many
SELECT id
FROM organization_members
WHERE organization_id = $1
  AND role = 'owner'
FOR UPDATE;

-- name: OrganizationExists :one
SELECT EXISTS (
    SELECT 1
    FROM organizations
    WHERE id = $1
      AND deleted_at IS NULL
) AS exists;

-- name: UserExists :one
SELECT EXISTS (
    SELECT 1
    FROM users
    WHERE id = $1
) AS exists;
