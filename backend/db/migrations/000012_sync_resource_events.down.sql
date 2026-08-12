DROP INDEX IF EXISTS idx_workflow_events_repository_started_at;

DROP TABLE IF EXISTS workflow_events;

DROP INDEX IF EXISTS idx_deployments_github_deployment_id;

ALTER TABLE deployments
    DROP COLUMN IF EXISTS github_deployment_id;
