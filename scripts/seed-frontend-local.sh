#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-devlens-postgres-1}"
CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-devlens-clickhouse-1}"

ORG_ID="e6362d4f-06a1-bf46-3198-35ef6d7433b3"
INSTALLATION_DB_ID="1f77a4ee-7d1f-4ef2-86d2-4a6c09d40b11"
INSTALLATION_ID="900001001"
SYNCED_REPO_ID="5b7d876d-6b80-4a53-9f8d-3d4e8c2f9a11"
SYNCED_REPO_GITHUB_ID="900000201"
SYNCED_REPO_INSTALLATION_DB_ID="e0120a51-93d6-4fd6-96d0-a1b35f7d0e11"
SYNCED_REPO_SYNC_JOB_ID="52b3c61f-0e6d-4db6-8f5e-7a226db2a811"

PG_PR1_ID="9a16d5c5-cf0b-4205-a06e-7d7d8ee4b101"
PG_PR2_ID="9a16d5c5-cf0b-4205-a06e-7d7d8ee4b102"
PG_PR3_ID="9a16d5c5-cf0b-4205-a06e-7d7d8ee4b103"
PG_REVIEW1_ID="2c5e87d2-5d5e-4df0-b6ac-447cb283c101"
PG_REVIEW2_ID="2c5e87d2-5d5e-4df0-b6ac-447cb283c102"
PG_REVIEW3_ID="2c5e87d2-5d5e-4df0-b6ac-447cb283c103"

echo "Seeding PostgreSQL onboarding and sync state..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<SQL
BEGIN;

INSERT INTO github_installations (
    id,
    organization_id,
    installation_id,
    installed_at,
    account_login,
    account_type,
    target_type,
    status,
    permissions_json,
    installed_by_github_user_id,
    updated_at,
    disconnected_at,
    suspended_at
) VALUES (
    '${INSTALLATION_DB_ID}',
    '${ORG_ID}',
    ${INSTALLATION_ID},
    '2026-08-13 10:55:00+00',
    'local-devlens',
    'Organization',
    'selected_repositories',
    'connected',
    '{"actions":"read","contents":"read","deployments":"read","metadata":"read","pull_requests":"read"}'::jsonb,
    900000010,
    '2026-08-13 10:55:00+00',
    NULL,
    NULL
)
ON CONFLICT (organization_id) DO UPDATE SET
    installation_id = EXCLUDED.installation_id,
    account_login = EXCLUDED.account_login,
    account_type = EXCLUDED.account_type,
    target_type = EXCLUDED.target_type,
    status = EXCLUDED.status,
    permissions_json = EXCLUDED.permissions_json,
    installed_by_github_user_id = EXCLUDED.installed_by_github_user_id,
    updated_at = EXCLUDED.updated_at,
    disconnected_at = NULL,
    suspended_at = NULL;

INSERT INTO repositories (
    id,
    organization_id,
    github_id,
    name,
    full_name,
    default_branch,
    is_active,
    last_synced_at,
    created_at,
    updated_at,
    archived_at,
    github_installation_repository_id,
    initial_sync_status,
    initial_sync_completed_at,
    sync_error_message
) VALUES (
    '${SYNCED_REPO_ID}',
    '${ORG_ID}',
    ${SYNCED_REPO_GITHUB_ID},
    'local-devlens-api',
    'local-devlens/local-devlens-api',
    'main',
    TRUE,
    '2026-08-13 11:08:00+00',
    '2026-08-13 10:56:00+00',
    '2026-08-13 11:08:00+00',
    NULL,
    NULL,
    'synced',
    '2026-08-13 11:08:00+00',
    NULL
)
ON CONFLICT (id) DO UPDATE SET
    organization_id = EXCLUDED.organization_id,
    github_id = EXCLUDED.github_id,
    name = EXCLUDED.name,
    full_name = EXCLUDED.full_name,
    default_branch = EXCLUDED.default_branch,
    is_active = EXCLUDED.is_active,
    last_synced_at = EXCLUDED.last_synced_at,
    updated_at = EXCLUDED.updated_at,
    archived_at = NULL,
    initial_sync_status = EXCLUDED.initial_sync_status,
    initial_sync_completed_at = EXCLUDED.initial_sync_completed_at,
    sync_error_message = NULL;

INSERT INTO github_installation_repositories (
    id,
    github_installation_id,
    github_repository_id,
    name,
    owner_login,
    full_name,
    private,
    default_branch,
    installation_status,
    selection_status,
    permissions_json,
    linked_repository_id,
    last_synced_at,
    sync_error_message,
    created_at,
    updated_at,
    last_discovered_at
) VALUES (
    '${SYNCED_REPO_INSTALLATION_DB_ID}',
    '${INSTALLATION_DB_ID}',
    ${SYNCED_REPO_GITHUB_ID},
    'local-devlens-api',
    'local-devlens',
    'local-devlens/local-devlens-api',
    FALSE,
    'main',
    'accessible',
    'selected',
    '{"actions":"read","contents":"read","deployments":"read","metadata":"read","pull_requests":"read"}'::jsonb,
    '${SYNCED_REPO_ID}',
    '2026-08-13 11:08:00+00',
    NULL,
    '2026-08-13 10:56:00+00',
    '2026-08-13 11:08:00+00',
    '2026-08-13 10:56:00+00'
)
ON CONFLICT (github_installation_id, github_repository_id) DO UPDATE SET
    name = EXCLUDED.name,
    owner_login = EXCLUDED.owner_login,
    full_name = EXCLUDED.full_name,
    private = EXCLUDED.private,
    default_branch = EXCLUDED.default_branch,
    installation_status = EXCLUDED.installation_status,
    selection_status = EXCLUDED.selection_status,
    permissions_json = EXCLUDED.permissions_json,
    linked_repository_id = EXCLUDED.linked_repository_id,
    last_synced_at = EXCLUDED.last_synced_at,
    sync_error_message = NULL,
    updated_at = EXCLUDED.updated_at,
    last_discovered_at = EXCLUDED.last_discovered_at;

UPDATE repositories
SET github_installation_repository_id = '${SYNCED_REPO_INSTALLATION_DB_ID}',
    initial_sync_status = 'synced',
    initial_sync_completed_at = '2026-08-13 11:08:00+00',
    sync_error_message = NULL,
    last_synced_at = '2026-08-13 11:08:00+00',
    updated_at = '2026-08-13 11:08:00+00'
WHERE id = '${SYNCED_REPO_ID}';

INSERT INTO sync_jobs (
    id,
    repository_id,
    job_type,
    status,
    progress,
    idempotency_key,
    triggered_by,
    error_message,
    error_code,
    started_at,
    finished_at,
    created_at,
    updated_at
) VALUES (
    '${SYNCED_REPO_SYNC_JOB_ID}',
    '${SYNCED_REPO_ID}',
    'repository_sync',
    'completed',
    100,
    'frontend-demo-sync',
    NULL,
    NULL,
    NULL,
    '2026-08-13 11:06:00+00',
    '2026-08-13 11:08:00+00',
    '2026-08-13 11:05:00+00',
    '2026-08-13 11:08:00+00'
)
ON CONFLICT (id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    status = EXCLUDED.status,
    progress = EXCLUDED.progress,
    error_message = NULL,
    error_code = NULL,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at,
    updated_at = EXCLUDED.updated_at;

INSERT INTO pull_requests (
    id,
    repository_id,
    github_pr_id,
    number,
    title,
    author,
    state,
    created_at,
    merged_at,
    closed_at,
    additions,
    deletions,
    files_changed,
    is_draft
) VALUES
(
    '${PG_PR1_ID}',
    '${SYNCED_REPO_ID}',
    900001101,
    11,
    'Seed dashboard metrics data',
    'alice',
    'closed',
    '2026-08-01 09:00:00+00',
    '2026-08-01 13:00:00+00',
    '2026-08-01 13:00:00+00',
    180,
    30,
    8,
    FALSE
),
(
    '${PG_PR2_ID}',
    '${SYNCED_REPO_ID}',
    900001102,
    12,
    'Add live metrics fixtures',
    'bob',
    'closed',
    '2026-08-02 08:30:00+00',
    '2026-08-02 10:30:00+00',
    '2026-08-02 10:30:00+00',
    40,
    10,
    4,
    FALSE
),
(
    '${PG_PR3_ID}',
    '${SYNCED_REPO_ID}',
    900001103,
    13,
    'Open review queue example',
    'carol',
    'open',
    '2026-08-04 07:00:00+00',
    NULL,
    NULL,
    20,
    5,
    2,
    FALSE
)
ON CONFLICT (id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    github_pr_id = EXCLUDED.github_pr_id,
    number = EXCLUDED.number,
    title = EXCLUDED.title,
    author = EXCLUDED.author,
    state = EXCLUDED.state,
    created_at = EXCLUDED.created_at,
    merged_at = EXCLUDED.merged_at,
    closed_at = EXCLUDED.closed_at,
    additions = EXCLUDED.additions,
    deletions = EXCLUDED.deletions,
    files_changed = EXCLUDED.files_changed,
    is_draft = EXCLUDED.is_draft;

INSERT INTO pull_request_reviews (
    id,
    pull_request_id,
    github_review_id,
    reviewer,
    review_requested_at,
    first_review_at,
    review_submitted_at,
    state
) VALUES
(
    '${PG_REVIEW1_ID}',
    '${PG_PR1_ID}',
    910001101,
    'dave',
    '2026-08-01 09:30:00+00',
    '2026-08-01 10:00:00+00',
    '2026-08-01 10:15:00+00',
    'APPROVED'
),
(
    '${PG_REVIEW2_ID}',
    '${PG_PR2_ID}',
    910001102,
    'erin',
    '2026-08-02 08:45:00+00',
    '2026-08-02 09:15:00+00',
    '2026-08-02 09:30:00+00',
    'COMMENTED'
),
(
    '${PG_REVIEW3_ID}',
    '${PG_PR3_ID}',
    910001103,
    'frank',
    '2026-08-04 07:15:00+00',
    NULL,
    NULL,
    'PENDING'
)
ON CONFLICT (id) DO UPDATE SET
    pull_request_id = EXCLUDED.pull_request_id,
    github_review_id = EXCLUDED.github_review_id,
    reviewer = EXCLUDED.reviewer,
    review_requested_at = EXCLUDED.review_requested_at,
    first_review_at = EXCLUDED.first_review_at,
    review_submitted_at = EXCLUDED.review_submitted_at,
    state = EXCLUDED.state;

COMMIT;
SQL

echo "Seeding ClickHouse dashboard and metrics fixtures..."
docker exec -i "${CLICKHOUSE_CONTAINER}" clickhouse-client --user devlens --password devlens --multiquery <<SQL
INSERT INTO devlens.metrics_daily (
    repository_id,
    metric_date,
    pr_cycle_time_minutes,
    review_wait_minutes,
    average_review_minutes,
    average_files_changed,
    average_additions,
    average_deletions,
    deployment_frequency,
    change_failure_rate,
    review_coverage,
    pr_count,
    merged_pr_count,
    reviewed_pr_count,
    review_wait_sample_count,
    review_time_sample_count,
    successful_deployment_count,
    failed_deployment_count,
    calculated_at,
    metric_version
) VALUES
(
    '${SYNCED_REPO_ID}',
    '2026-08-01',
    240,
    90,
    45,
    8,
    150,
    30,
    1,
    0,
    1,
    2,
    2,
    2,
    2,
    2,
    1,
    0,
    '2026-08-13 11:08:00.000',
    1
),
(
    '${SYNCED_REPO_ID}',
    '2026-08-02',
    120,
    60,
    30,
    4,
    40,
    10,
    0,
    1,
    1,
    1,
    1,
    1,
    1,
    1,
    0,
    1,
    '2026-08-13 11:08:00.000',
    1
),
(
    '${SYNCED_REPO_ID}',
    '2026-08-04',
    0,
    0,
    0,
    2,
    20,
    5,
    0,
    0,
    0,
    0,
    0,
    0,
    0,
    0,
    0,
    0,
    '2026-08-13 11:08:00.000',
    1
);

INSERT INTO devlens.pull_requests (
    id,
    repository_id,
    github_pr_id,
    number,
    title,
    author,
    state,
    created_at,
    merged_at,
    closed_at,
    additions,
    deletions,
    files_changed,
    is_draft,
    synced_at
) VALUES
(
    'ch-pr-900001101',
    '${SYNCED_REPO_ID}',
    900001101,
    11,
    'Seed dashboard metrics data',
    'alice',
    'closed',
    '2026-08-01 09:00:00.000',
    '2026-08-01 13:00:00.000',
    '2026-08-01 13:00:00.000',
    180,
    30,
    8,
    0,
    '2026-08-13 11:08:00.000'
),
(
    'ch-pr-900001102',
    '${SYNCED_REPO_ID}',
    900001102,
    12,
    'Add live metrics fixtures',
    'bob',
    'closed',
    '2026-08-02 08:30:00.000',
    '2026-08-02 10:30:00.000',
    '2026-08-02 10:30:00.000',
    40,
    10,
    4,
    0,
    '2026-08-13 11:08:00.000'
),
(
    'ch-pr-900001103',
    '${SYNCED_REPO_ID}',
    900001103,
    13,
    'Open review queue example',
    'carol',
    'open',
    '2026-08-04 07:00:00.000',
    NULL,
    NULL,
    20,
    5,
    2,
    0,
    '2026-08-13 11:08:00.000'
);

INSERT INTO devlens.file_changes (
    id,
    pull_request_id,
    file_path,
    additions,
    deletions,
    commit_count,
    synced_at
) VALUES
(
    'ch-fc-1',
    'ch-pr-900001101',
    'backend/internal/metrics/service.go',
    60,
    10,
    3,
    '2026-08-13 11:08:00.000'
),
(
    'ch-fc-2',
    'ch-pr-900001101',
    'backend/internal/httpapi/sync_job_handler.go',
    30,
    15,
    2,
    '2026-08-13 11:08:00.000'
),
(
    'ch-fc-3',
    'ch-pr-900001102',
    'frontend/src/dashboard.tsx',
    20,
    5,
    1,
    '2026-08-13 11:08:00.000'
),
(
    'ch-fc-4',
    'ch-pr-900001103',
    'frontend/src/widgets/review-queue.tsx',
    15,
    3,
    1,
    '2026-08-13 11:08:00.000'
);

INSERT INTO devlens.deployments (
    id,
    repository_id,
    environment,
    status,
    deployed_at,
    synced_at
) VALUES
(
    'ch-deploy-1',
    '${SYNCED_REPO_ID}',
    'production',
    'success',
    '2026-08-01 15:00:00.000',
    '2026-08-13 11:08:00.000'
),
(
    'ch-deploy-2',
    '${SYNCED_REPO_ID}',
    'production',
    'failed',
    '2026-08-02 16:00:00.000',
    '2026-08-13 11:08:00.000'
);
SQL

echo "Seed complete."
echo "Organization: ${ORG_ID}"
echo "Synced repository: ${SYNCED_REPO_ID} (local-devlens/local-devlens-api)"
