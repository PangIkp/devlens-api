DROP INDEX IF EXISTS idx_github_installation_repositories_linked_repository;
DROP INDEX IF EXISTS idx_github_installation_repositories_installation_repo;
DROP INDEX IF EXISTS idx_github_installations_organization_unique;

DROP TABLE IF EXISTS github_installation_repositories;

ALTER TABLE github_installations
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS suspended_at,
    DROP COLUMN IF EXISTS installed_by_github_user_id,
    DROP COLUMN IF EXISTS permissions_json,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS target_type,
    DROP COLUMN IF EXISTS account_type,
    DROP COLUMN IF EXISTS account_login;
