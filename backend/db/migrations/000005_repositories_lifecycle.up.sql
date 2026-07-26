ALTER TABLE repositories
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN updated_at TIMESTAMPTZ,
    ADD COLUMN archived_at TIMESTAMPTZ;

ALTER TABLE repositories
    ALTER COLUMN github_id SET NOT NULL,
    ALTER COLUMN name SET NOT NULL,
    ALTER COLUMN full_name SET NOT NULL;

CREATE UNIQUE INDEX idx_repositories_full_name_unique
    ON repositories (full_name);

