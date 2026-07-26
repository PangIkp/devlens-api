DROP INDEX IF EXISTS idx_repositories_full_name_unique;

ALTER TABLE repositories
    ALTER COLUMN full_name DROP NOT NULL,
    ALTER COLUMN name DROP NOT NULL,
    ALTER COLUMN github_id DROP NOT NULL;

ALTER TABLE repositories
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at;

