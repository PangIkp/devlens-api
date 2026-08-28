DROP INDEX IF EXISTS idx_github_installations_account_identity;

ALTER TABLE github_installations
    DROP COLUMN IF EXISTS account_github_id;