-- name: GetOrganizationRetentionSettings :one
SELECT organization_id,
       analytics_raw_retention_days,
       created_at,
       updated_at,
       updated_by
FROM organization_retention_settings
WHERE organization_id = $1;

-- name: UpsertOrganizationRetentionSettings :one
INSERT INTO organization_retention_settings (
    organization_id, analytics_raw_retention_days, created_at, updated_at, updated_by
) VALUES (
    $1, $2, NOW(), NOW(), $3
)
ON CONFLICT (organization_id) DO UPDATE SET
    analytics_raw_retention_days = EXCLUDED.analytics_raw_retention_days,
    updated_at = NOW(),
    updated_by = EXCLUDED.updated_by
RETURNING organization_id,
          analytics_raw_retention_days,
          created_at,
          updated_at,
          updated_by;
