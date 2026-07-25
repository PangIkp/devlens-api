ALTER TABLE organizations
    ADD COLUMN slug VARCHAR(255),
    ADD COLUMN updated_at TIMESTAMPTZ,
    ADD COLUMN deleted_at TIMESTAMPTZ;

UPDATE organizations
SET slug = LOWER(REGEXP_REPLACE(COALESCE(NULLIF(name, ''), 'org-' || github_id::text), '[^a-zA-Z0-9]+', '-', 'g'));

UPDATE organizations
SET slug = TRIM(BOTH '-' FROM slug);

UPDATE organizations
SET slug = 'org-' || github_id::text
WHERE slug IS NULL OR slug = '';

WITH duplicate_slugs AS (
    SELECT
        id,
        ROW_NUMBER() OVER (PARTITION BY slug ORDER BY created_at, id) AS row_num
    FROM organizations
)
UPDATE organizations AS o
SET slug = o.slug || '-' || SUBSTRING(REPLACE(o.id::text, '-', '') FROM 1 FOR 8)
FROM duplicate_slugs AS d
WHERE o.id = d.id
  AND d.row_num > 1;

ALTER TABLE organizations
    ALTER COLUMN slug SET NOT NULL;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_github_id_key;

CREATE UNIQUE INDEX idx_organizations_github_id_active
    ON organizations (github_id)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX idx_organizations_slug_active
    ON organizations (slug)
    WHERE deleted_at IS NULL;
