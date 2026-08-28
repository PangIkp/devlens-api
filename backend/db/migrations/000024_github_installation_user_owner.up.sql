ALTER TABLE github_installations
    ADD COLUMN IF NOT EXISTS connected_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

UPDATE github_installations gi
SET connected_by_user_id = (
    SELECT om.user_id
    FROM organization_members om
    WHERE om.organization_id = gi.organization_id
    ORDER BY
        CASE om.role
            WHEN 'owner' THEN 0
            WHEN 'admin' THEN 1
            ELSE 2
        END,
        om.user_id::text
    LIMIT 1
)
WHERE gi.connected_by_user_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM organization_members om
      WHERE om.organization_id = gi.organization_id
  );

CREATE INDEX IF NOT EXISTS idx_github_installations_connected_by_user_id
    ON github_installations (connected_by_user_id);