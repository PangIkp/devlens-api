DROP INDEX IF EXISTS idx_file_changes_pull_request_id;
DROP TABLE IF EXISTS file_changes;

DROP INDEX IF EXISTS idx_deployments_repository_id_deployed_at;
DROP TABLE IF EXISTS deployments;

ALTER TABLE pull_requests
    DROP COLUMN IF EXISTS is_draft;
