DROP INDEX IF EXISTS idx_organization_members_org_user_unique;

ALTER TABLE organization_members
    ALTER COLUMN role DROP NOT NULL;
