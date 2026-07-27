UPDATE organization_members
SET role = 'member'
WHERE role IS NULL OR role = '';

ALTER TABLE organization_members
    ALTER COLUMN role SET NOT NULL;

CREATE UNIQUE INDEX idx_organization_members_org_user_unique
    ON organization_members (organization_id, user_id);
