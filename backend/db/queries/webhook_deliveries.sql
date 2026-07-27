-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (
    id,
    repository_id,
    github_delivery_id,
    event_type,
    processed,
    received_at,
    action,
    payload,
    sync_job_id,
    updated_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    NOW(),
    $6,
    $7,
    $8,
    $9
)
ON CONFLICT (github_delivery_id) DO NOTHING
RETURNING id, repository_id, github_delivery_id, event_type, processed, received_at, action, payload, sync_job_id, updated_at;

-- name: GetWebhookDeliveryByGithubDeliveryID :one
SELECT id, repository_id, github_delivery_id, event_type, processed, received_at, action, payload, sync_job_id, updated_at
FROM webhook_deliveries
WHERE github_delivery_id = $1;

-- name: MarkWebhookDeliveryProcessed :one
UPDATE webhook_deliveries
SET processed = $2,
    sync_job_id = $3,
    updated_at = $4
WHERE id = $1
RETURNING id, repository_id, github_delivery_id, event_type, processed, received_at, action, payload, sync_job_id, updated_at;
