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
