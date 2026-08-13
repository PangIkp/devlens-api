-- name: GetOrganizationRuleSettings :one
SELECT organization_id,
       config_json,
       created_at,
       updated_at,
       updated_by
FROM organization_rule_settings
WHERE organization_id = $1;

-- name: UpsertOrganizationRuleSettings :one
INSERT INTO organization_rule_settings (
    organization_id, config_json, created_at, updated_at, updated_by
) VALUES (
    $1, $2, NOW(), NOW(), $3
)
ON CONFLICT (organization_id) DO UPDATE SET
    config_json = EXCLUDED.config_json,
    updated_at = NOW(),
    updated_by = EXCLUDED.updated_by
RETURNING organization_id,
          config_json,
          created_at,
          updated_at,
          updated_by;
