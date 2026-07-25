DROP INDEX IF EXISTS idx_organizations_slug_active;
DROP INDEX IF EXISTS idx_organizations_github_id_active;

ALTER TABLE organizations
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS slug;

ALTER TABLE organizations
    ADD CONSTRAINT organizations_github_id_key UNIQUE (github_id);
