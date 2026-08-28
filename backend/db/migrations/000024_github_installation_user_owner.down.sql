DROP INDEX IF EXISTS idx_github_installations_connected_by_user_id;

ALTER TABLE github_installations
    DROP COLUMN IF EXISTS connected_by_user_id;