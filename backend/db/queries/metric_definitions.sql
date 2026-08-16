-- name: GetActiveMetricDefinitionByKey :one
SELECT id,
       metric_key,
       name,
       metric_version,
       algorithm_version,
       unit,
       description,
       config_json,
       is_active,
       created_at,
       created_by,
       updated_at,
       updated_by,
       deleted_at,
       deleted_by
FROM metric_definitions
WHERE metric_key = $1
  AND is_active = TRUE
  AND deleted_at IS NULL
ORDER BY metric_version DESC, created_at DESC
LIMIT 1;
