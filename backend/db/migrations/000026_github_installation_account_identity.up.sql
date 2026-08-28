ALTER TABLE github_installations
    ADD COLUMN IF NOT EXISTS account_github_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_github_installations_account_identity
    ON github_installations (account_github_id, lower(account_login), account_type);