#!/usr/bin/env bash
#
# Seeds a large, realistic dataset (85 PRs across 3 repos, reviews, file
# changes, deployments) into the "test01" organization already owned by
# local@devlens.test, tuned to trigger every insight rule type:
# large_pr_detection, slow_review_detection, hotspot_detection,
# bottleneck_detection, review_concentration, deployment_failure_trend.
#
# Idempotent: safe to re-run, every row has a deterministic id.

set -euo pipefail

POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-devlens-postgres-1}"
CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-devlens-clickhouse-1}"

ORG_ID="e5cdaeab-5972-05c0-8499-d7b7e255a9d2" # test01, owned by local@devlens.test
INSTALLATION_ID="22222222-2222-4222-8222-200000000001"

echo "Seeding installation, repositories, sync jobs..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<SQL
BEGIN;

INSERT INTO github_installations (
    id, organization_id, installation_id, installed_at, account_login,
    account_type, target_type, status, permissions_json,
    installed_by_github_user_id, updated_at, disconnected_at, suspended_at
) VALUES (
    '${INSTALLATION_ID}', '${ORG_ID}', 900020001, NOW() - interval '20 days',
    'local-demo', 'Organization', 'selected_repositories', 'connected',
    '{"actions":"read","contents":"read","deployments":"read","metadata":"read","pull_requests":"read"}'::jsonb,
    900000099, NOW(), NULL, NULL
)
ON CONFLICT (organization_id) DO UPDATE SET
    installation_id = EXCLUDED.installation_id,
    status = EXCLUDED.status,
    disconnected_at = NULL,
    suspended_at = NULL,
    updated_at = NOW();

INSERT INTO repositories (
    id, organization_id, github_id, name, full_name, default_branch,
    is_active, last_synced_at, created_at, updated_at, archived_at,
    initial_sync_status, initial_sync_completed_at, sync_error_message
) VALUES
    ('11111111-1111-4111-8111-100000000001', '${ORG_ID}', 951000000, 'api-gateway', 'local-demo/api-gateway', 'main', TRUE, NOW(), NOW() - interval '20 days', NOW(), NULL, 'synced', NOW(), NULL),
    ('11111111-1111-4111-8111-100000000002', '${ORG_ID}', 952000000, 'web-console', 'local-demo/web-console', 'main', TRUE, NOW(), NOW() - interval '20 days', NOW(), NULL, 'synced', NOW(), NULL),
    ('11111111-1111-4111-8111-100000000003', '${ORG_ID}', 953000000, 'data-pipeline', 'local-demo/data-pipeline', 'main', TRUE, NOW(), NOW() - interval '20 days', NOW(), NULL, 'synced', NOW(), NULL)
ON CONFLICT (id) DO UPDATE SET
    is_active = TRUE, last_synced_at = NOW(), updated_at = NOW(),
    initial_sync_status = 'synced', initial_sync_completed_at = NOW(), sync_error_message = NULL;

INSERT INTO github_installation_repositories (
    id, github_installation_id, github_repository_id, name, owner_login,
    full_name, private, default_branch, installation_status, selection_status,
    permissions_json, linked_repository_id, last_synced_at, sync_error_message,
    created_at, updated_at, last_discovered_at
) VALUES
    ('33333333-3333-4333-8333-300000000001', '${INSTALLATION_ID}', 951000000, 'api-gateway', 'local-demo', 'local-demo/api-gateway', FALSE, 'main', 'accessible', 'selected', '{}'::jsonb, '11111111-1111-4111-8111-100000000001', NOW(), NULL, NOW() - interval '20 days', NOW(), NOW()),
    ('33333333-3333-4333-8333-300000000002', '${INSTALLATION_ID}', 952000000, 'web-console', 'local-demo', 'local-demo/web-console', FALSE, 'main', 'accessible', 'selected', '{}'::jsonb, '11111111-1111-4111-8111-100000000002', NOW(), NULL, NOW() - interval '20 days', NOW(), NOW()),
    ('33333333-3333-4333-8333-300000000003', '${INSTALLATION_ID}', 953000000, 'data-pipeline', 'local-demo', 'local-demo/data-pipeline', FALSE, 'main', 'accessible', 'selected', '{}'::jsonb, '11111111-1111-4111-8111-100000000003', NOW(), NULL, NOW() - interval '20 days', NOW(), NOW())
ON CONFLICT (github_installation_id, github_repository_id) DO UPDATE SET
    installation_status = 'accessible', selection_status = 'selected',
    linked_repository_id = EXCLUDED.linked_repository_id, updated_at = NOW();

INSERT INTO sync_jobs (id, repository_id, job_type, status, progress, idempotency_key, started_at, finished_at, created_at, updated_at)
VALUES
    ('44444444-4444-4444-8444-400000000001', '11111111-1111-4111-8111-100000000001', 'repository_sync', 'completed', 100, 'rich-seed-api-gateway', NOW() - interval '19 days', NOW() - interval '19 days' + interval '2 hours', NOW() - interval '19 days', NOW()),
    ('44444444-4444-4444-8444-400000000002', '11111111-1111-4111-8111-100000000002', 'repository_sync', 'completed', 100, 'rich-seed-web-console', NOW() - interval '19 days', NOW() - interval '19 days' + interval '2 hours', NOW() - interval '19 days', NOW()),
    ('44444444-4444-4444-8444-400000000003', '11111111-1111-4111-8111-100000000003', 'repository_sync', 'completed', 100, 'rich-seed-data-pipeline', NOW() - interval '19 days', NOW() - interval '19 days' + interval '2 hours', NOW() - interval '19 days', NOW())
ON CONFLICT (id) DO UPDATE SET status = 'completed', progress = 100, finished_at = EXCLUDED.finished_at, updated_at = NOW();

COMMIT;
SQL

echo "Seeding pull requests (repo A: api-gateway, 45 PRs)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_requests (id, repository_id, github_pr_id, number, title, author, state, created_at, merged_at, closed_at, additions, deletions, files_changed, is_draft)
SELECT
    ('55550001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000001',
    951000000 + n,
    n,
    'Improve gateway request routing #' || n,
    (ARRAY['alice','bob','carol','dan','erin','frank'])[(n % 6) + 1],
    CASE WHEN n % 10 IN (6,7) THEN 'closed' WHEN n % 10 IN (8,9) THEN 'open' ELSE 'merged' END,
    created_at,
    CASE WHEN n % 10 IN (6,7,8,9) THEN NULL ELSE created_at + make_interval(hours => 24 + (n * 7) % 168) END,
    CASE WHEN n % 10 IN (6,7) THEN created_at + make_interval(hours => 2 + n % 48) ELSE NULL END,
    additions, deletions, files_changed, FALSE
FROM (
    SELECT
        n,
        NOW() - make_interval(days => n % 20, mins => (n * 37) % 1440) AS created_at,
        CASE
            WHEN n <= 3 THEN 900 + n * 50
            WHEN n <= 8 THEN 300 + n * 30
            ELSE 10 + (n * 7) % 200
        END AS additions,
        CASE
            WHEN n <= 3 THEN 300 + n * 20
            WHEN n <= 8 THEN 150 + n * 10
            ELSE 5 + (n * 3) % 80
        END AS deletions,
        CASE
            WHEN n <= 3 THEN 55 + n
            WHEN n <= 8 THEN 28 + n
            ELSE 1 + n % 12
        END AS files_changed
    FROM generate_series(1, 45) AS n
) AS gen
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state, merged_at = EXCLUDED.merged_at, closed_at = EXCLUDED.closed_at,
    additions = EXCLUDED.additions, deletions = EXCLUDED.deletions, files_changed = EXCLUDED.files_changed;

COMMIT;
SQL

echo "Seeding pull requests (repo B: web-console, 25 PRs)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_requests (id, repository_id, github_pr_id, number, title, author, state, created_at, merged_at, closed_at, additions, deletions, files_changed, is_draft)
SELECT
    ('55550002-bbbb-4bbb-8bbb-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000002',
    952000000 + n,
    n,
    'Web console update #' || n,
    (ARRAY['grace','henry','ivan','june','kim'])[(n % 5) + 1],
    CASE WHEN n % 10 IN (7,8) THEN 'closed' WHEN n % 10 = 9 THEN 'open' ELSE 'merged' END,
    created_at,
    CASE WHEN n % 10 IN (7,8,9) THEN NULL ELSE created_at + make_interval(hours => 3 + (n * 5) % 60) END,
    CASE WHEN n % 10 IN (7,8) THEN created_at + make_interval(hours => 2 + n % 30) ELSE NULL END,
    additions, deletions, files_changed, FALSE
FROM (
    SELECT
        n,
        NOW() - make_interval(days => n % 15, mins => (n * 53) % 1440) AS created_at,
        CASE WHEN n % 9 = 0 THEN 500 ELSE 10 + (n * 11) % 150 END AS additions,
        CASE WHEN n % 9 = 0 THEN 200 ELSE 5 + (n * 5) % 60 END AS deletions,
        CASE WHEN n % 9 = 0 THEN 27 ELSE 1 + n % 8 END AS files_changed
    FROM generate_series(1, 25) AS n
) AS gen
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state, merged_at = EXCLUDED.merged_at, closed_at = EXCLUDED.closed_at,
    additions = EXCLUDED.additions, deletions = EXCLUDED.deletions, files_changed = EXCLUDED.files_changed;

COMMIT;
SQL

echo "Seeding pull requests (repo C: data-pipeline, 15 PRs)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_requests (id, repository_id, github_pr_id, number, title, author, state, created_at, merged_at, closed_at, additions, deletions, files_changed, is_draft)
SELECT
    ('55550003-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000003',
    953000000 + n,
    n,
    'Data pipeline stage #' || n,
    (ARRAY['liam','maya','noah'])[(n % 3) + 1],
    CASE WHEN n % 5 = 3 THEN 'closed' WHEN n % 5 = 4 THEN 'open' ELSE 'merged' END,
    created_at,
    CASE WHEN n % 5 IN (3,4) THEN NULL ELSE created_at + make_interval(hours => 4 + (n * 9) % 40) END,
    CASE WHEN n % 5 = 3 THEN created_at + make_interval(hours => 2 + n % 20) ELSE NULL END,
    20 + (n * 9) % 180, 10 + (n * 4) % 70, 1 + n % 10, FALSE
FROM (SELECT n, NOW() - make_interval(days => n % 18) AS created_at FROM generate_series(1, 15) AS n) AS gen
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state, merged_at = EXCLUDED.merged_at, closed_at = EXCLUDED.closed_at;

COMMIT;
SQL

echo "Seeding reviews (repo A: spread reviewers + slow-review examples)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_request_reviews (id, pull_request_id, github_review_id, reviewer, review_requested_at, first_review_at, review_submitted_at, state)
SELECT
    ('66660001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    ('55550001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    961000000 + n,
    (ARRAY['dave','erin_r','grace_r','henry_r','ivan_r'])[(n % 5) + 1],
    requested_at,
    CASE WHEN is_pending THEN NULL ELSE requested_at + make_interval(hours => wait_hours) END,
    CASE WHEN is_pending THEN NULL ELSE requested_at + make_interval(hours => wait_hours) END,
    CASE WHEN is_pending THEN 'PENDING' WHEN n % 4 = 0 THEN 'APPROVED' WHEN n % 4 = 1 THEN 'CHANGES_REQUESTED' ELSE 'COMMENTED' END
FROM (
    SELECT
        n,
        pr.created_at + interval '1 hour' AS requested_at,
        n % 13 = 0 AS is_pending,
        CASE WHEN n % 7 = 0 THEN 30 + (n % 60) ELSE 1 + (n % 18) END AS wait_hours
    FROM generate_series(1, 45) AS n
    INNER JOIN pull_requests pr ON pr.id = ('55550001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid
    WHERE n % 10 != 9
) AS gen
ON CONFLICT (id) DO UPDATE SET
    first_review_at = EXCLUDED.first_review_at, review_submitted_at = EXCLUDED.review_submitted_at, state = EXCLUDED.state;

COMMIT;
SQL

echo "Seeding reviews (repo B: concentrated on alex-lead for review_concentration)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_request_reviews (id, pull_request_id, github_review_id, reviewer, review_requested_at, first_review_at, review_submitted_at, state)
SELECT
    ('66660002-bbbb-4bbb-8bbb-' || lpad(n::text, 12, '0'))::uuid,
    ('55550002-bbbb-4bbb-8bbb-' || lpad(n::text, 12, '0'))::uuid,
    962000000 + n,
    CASE WHEN n % 4 IN (0,1,2) THEN 'alex-lead' ELSE (ARRAY['kim_r','noah_r','priya_r'])[(n % 3) + 1] END,
    requested_at,
    requested_at + make_interval(hours => 1 + (n % 10)),
    requested_at + make_interval(hours => 1 + (n % 10)),
    CASE n % 4 WHEN 0 THEN 'APPROVED' WHEN 1 THEN 'CHANGES_REQUESTED' WHEN 2 THEN 'COMMENTED' ELSE 'APPROVED' END
FROM (
    SELECT n, pr.created_at + interval '1 hour' AS requested_at
    FROM generate_series(1, 25) AS n
    INNER JOIN pull_requests pr ON pr.id = ('55550002-bbbb-4bbb-8bbb-' || lpad(n::text, 12, '0'))::uuid
    WHERE n % 9 != 0
) AS gen
ON CONFLICT (id) DO UPDATE SET
    first_review_at = EXCLUDED.first_review_at, review_submitted_at = EXCLUDED.review_submitted_at, state = EXCLUDED.state;

COMMIT;
SQL

echo "Seeding reviews (repo C: light coverage)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO pull_request_reviews (id, pull_request_id, github_review_id, reviewer, review_requested_at, first_review_at, review_submitted_at, state)
SELECT
    ('66660003-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    ('55550003-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    963000000 + n,
    (ARRAY['dave','erin_r','grace_r'])[(n % 3) + 1],
    requested_at,
    requested_at + make_interval(hours => 2 + (n % 15)),
    requested_at + make_interval(hours => 2 + (n % 15)),
    CASE n % 3 WHEN 0 THEN 'APPROVED' WHEN 1 THEN 'CHANGES_REQUESTED' ELSE 'COMMENTED' END
FROM (
    SELECT n, pr.created_at + interval '1 hour' AS requested_at
    FROM generate_series(1, 15) AS n
    INNER JOIN pull_requests pr ON pr.id = ('55550003-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid
    WHERE n % 3 != 0
) AS gen
ON CONFLICT (id) DO UPDATE SET
    first_review_at = EXCLUDED.first_review_at, review_submitted_at = EXCLUDED.review_submitted_at, state = EXCLUDED.state;

COMMIT;
SQL

echo "Seeding file changes (repo A: router.go / middleware.go as hotspots)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO file_changes (id, pull_request_id, file_path, additions, deletions, commit_count)
SELECT
    ('77770001-aaaa-4aaa-8aaa-' || lpad(n::text, 11, '0') || '1')::uuid,
    ('55550001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    CASE WHEN n % 3 = 0 THEN 'internal/gateway/router.go'
         WHEN n % 3 = 1 THEN 'internal/gateway/middleware.go'
         ELSE 'internal/gateway/handlers/handler_' || (n % 6) || '.go' END,
    30 + (n * 13) % 90, 10 + (n * 7) % 40, 2 + n % 5
FROM generate_series(1, 45) AS n
ON CONFLICT (id) DO UPDATE SET additions = EXCLUDED.additions, deletions = EXCLUDED.deletions, commit_count = EXCLUDED.commit_count;

INSERT INTO file_changes (id, pull_request_id, file_path, additions, deletions, commit_count)
SELECT
    ('77770001-aaaa-4aaa-8aaa-' || lpad(n::text, 11, '0') || '2')::uuid,
    ('55550001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    CASE WHEN n % 5 = 0 THEN 'internal/gateway/config.go' ELSE 'docs/notes_' || n || '.md' END,
    2 + (n % 8), n % 5, 1
FROM generate_series(1, 45) AS n
ON CONFLICT (id) DO UPDATE SET additions = EXCLUDED.additions, deletions = EXCLUDED.deletions;

INSERT INTO file_changes (id, pull_request_id, file_path, additions, deletions, commit_count)
SELECT
    ('77770002-bbbb-4bbb-8bbb-' || lpad(n::text, 11, '0') || '1')::uuid,
    ('55550002-bbbb-4bbb-8bbb-' || lpad(n::text, 12, '0'))::uuid,
    CASE WHEN n % 4 = 0 THEN 'src/app/store.ts' ELSE 'src/components/Widget_' || (n % 10) || '.tsx' END,
    10 + (n * 5) % 60, 5 + (n * 3) % 25, 1 + n % 3
FROM generate_series(1, 25) AS n
ON CONFLICT (id) DO UPDATE SET additions = EXCLUDED.additions, deletions = EXCLUDED.deletions;

INSERT INTO file_changes (id, pull_request_id, file_path, additions, deletions, commit_count)
SELECT
    ('77770003-cccc-4ccc-8ccc-' || lpad(n::text, 11, '0') || '1')::uuid,
    ('55550003-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    'pipelines/stage_' || (n % 5) || '.py',
    15 + (n * 6) % 70, 5 + (n * 2) % 30, 1 + n % 4
FROM generate_series(1, 15) AS n
ON CONFLICT (id) DO UPDATE SET additions = EXCLUDED.additions, deletions = EXCLUDED.deletions;

COMMIT;
SQL

echo "Seeding deployments (repo A production ~42% failed, repo C production ~60% failed + healthy staging)..."
docker exec -i "${POSTGRES_CONTAINER}" psql -U devlens -d devlens <<'SQL'
BEGIN;

INSERT INTO deployments (id, repository_id, environment, status, deployed_at)
SELECT
    ('88880001-aaaa-4aaa-8aaa-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000001',
    'production',
    CASE WHEN n % 12 IN (0,3,6,9,10) THEN 'failed' ELSE 'success' END,
    NOW() - make_interval(days => n % 18, hours => n)
FROM generate_series(1, 12) AS n
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

INSERT INTO deployments (id, repository_id, environment, status, deployed_at)
SELECT
    ('88880031-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000003',
    'production',
    CASE WHEN n % 10 IN (0,2,4,6,8,9) THEN 'failed' ELSE 'success' END,
    NOW() - make_interval(days => n % 15, hours => n)
FROM generate_series(1, 10) AS n
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

INSERT INTO deployments (id, repository_id, environment, status, deployed_at)
SELECT
    ('88880032-cccc-4ccc-8ccc-' || lpad(n::text, 12, '0'))::uuid,
    '11111111-1111-4111-8111-100000000003',
    'staging',
    CASE WHEN n % 8 = 0 THEN 'failed' ELSE 'success' END,
    NOW() - make_interval(days => n % 12, hours => n)
FROM generate_series(1, 8) AS n
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

COMMIT;
SQL

echo "Mirroring pull requests, file changes, and deployments into ClickHouse..."
docker exec -i "${CLICKHOUSE_CONTAINER}" clickhouse-client --user devlens --password devlens --multiquery <<'SQL'
INSERT INTO devlens.pull_requests
SELECT
    lower(concat('55550001-aaaa-4aaa-8aaa-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000001',
    951000000 + n,
    n,
    concat('Improve gateway request routing #', toString(n)),
    (['alice','bob','carol','dan','erin','frank'])[(n % 6) + 1],
    multiIf(n % 10 IN (6,7), 'closed', n % 10 IN (8,9), 'open', 'merged'),
    created_at,
    if(n % 10 IN (6,7,8,9), NULL, created_at + toIntervalHour(24 + (n * 7) % 168)),
    if(n % 10 IN (6,7), created_at + toIntervalHour(2 + n % 48), NULL),
    additions, deletions, files_changed, false, now()
FROM (
    SELECT
        number + 1 AS n,
        now() - toIntervalDay((number + 1) % 20) - toIntervalMinute(((number + 1) * 37) % 1440) AS created_at,
        multiIf(number + 1 <= 3, 900 + (number + 1) * 50, number + 1 <= 8, 300 + (number + 1) * 30, 10 + ((number + 1) * 7) % 200) AS additions,
        multiIf(number + 1 <= 3, 300 + (number + 1) * 20, number + 1 <= 8, 150 + (number + 1) * 10, 5 + ((number + 1) * 3) % 80) AS deletions,
        multiIf(number + 1 <= 3, 55 + (number + 1), number + 1 <= 8, 28 + (number + 1), 1 + (number + 1) % 12) AS files_changed
    FROM numbers(45)
);

INSERT INTO devlens.pull_requests
SELECT
    lower(concat('55550002-bbbb-4bbb-8bbb-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000002',
    952000000 + n,
    n,
    concat('Web console update #', toString(n)),
    (['grace','henry','ivan','june','kim'])[(n % 5) + 1],
    multiIf(n % 10 IN (7,8), 'closed', n % 10 = 9, 'open', 'merged'),
    created_at,
    if(n % 10 IN (7,8,9), NULL, created_at + toIntervalHour(3 + (n * 5) % 60)),
    if(n % 10 IN (7,8), created_at + toIntervalHour(2 + n % 30), NULL),
    additions, deletions, files_changed, false, now()
FROM (
    SELECT
        number + 1 AS n,
        now() - toIntervalDay((number + 1) % 15) - toIntervalMinute(((number + 1) * 53) % 1440) AS created_at,
        if((number + 1) % 9 = 0, 500, 10 + ((number + 1) * 11) % 150) AS additions,
        if((number + 1) % 9 = 0, 200, 5 + ((number + 1) * 5) % 60) AS deletions,
        if((number + 1) % 9 = 0, 27, 1 + (number + 1) % 8) AS files_changed
    FROM numbers(25)
);

INSERT INTO devlens.pull_requests
SELECT
    lower(concat('55550003-cccc-4ccc-8ccc-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000003',
    953000000 + n,
    n,
    concat('Data pipeline stage #', toString(n)),
    (['liam','maya','noah'])[(n % 3) + 1],
    multiIf(n % 5 = 3, 'closed', n % 5 = 4, 'open', 'merged'),
    created_at,
    if(n % 5 IN (3,4), NULL, created_at + toIntervalHour(4 + (n * 9) % 40)),
    if(n % 5 = 3, created_at + toIntervalHour(2 + n % 20), NULL),
    20 + (n * 9) % 180, 10 + (n * 4) % 70, 1 + n % 10, false, now()
FROM (SELECT number + 1 AS n, now() - toIntervalDay((number + 1) % 18) AS created_at FROM numbers(15));

INSERT INTO devlens.file_changes
SELECT
    lower(concat('77770001-aaaa-4aaa-8aaa-', leftPad(toString(n), 11, '0'), '1')),
    lower(concat('55550001-aaaa-4aaa-8aaa-', leftPad(toString(n), 12, '0'))),
    multiIf(n % 3 = 0, 'internal/gateway/router.go', n % 3 = 1, 'internal/gateway/middleware.go', concat('internal/gateway/handlers/handler_', toString(n % 6), '.go')),
    30 + (n * 13) % 90, 10 + (n * 7) % 40, 2 + n % 5, now()
FROM (SELECT number + 1 AS n FROM numbers(45));

INSERT INTO devlens.deployments
SELECT
    lower(concat('88880001-aaaa-4aaa-8aaa-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000001',
    'production',
    if(n % 12 IN (0,3,6,9,10), 'failed', 'success'),
    now() - toIntervalDay(n % 18) - toIntervalHour(n),
    now()
FROM (SELECT number + 1 AS n FROM numbers(12));

INSERT INTO devlens.deployments
SELECT
    lower(concat('88880031-cccc-4ccc-8ccc-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000003',
    'production',
    if(n % 10 IN (0,2,4,6,8,9), 'failed', 'success'),
    now() - toIntervalDay(n % 15) - toIntervalHour(n),
    now()
FROM (SELECT number + 1 AS n FROM numbers(10));

INSERT INTO devlens.deployments
SELECT
    lower(concat('88880032-cccc-4ccc-8ccc-', leftPad(toString(n), 12, '0'))),
    '11111111-1111-4111-8111-100000000003',
    'staging',
    if(n % 8 = 0, 'failed', 'success'),
    now() - toIntervalDay(n % 12) - toIntervalHour(n),
    now()
FROM (SELECT number + 1 AS n FROM numbers(8));
SQL

echo "Seed complete."
echo "Organization: ${ORG_ID} (test01, owned by local@devlens.test)"
echo "Repos: local-demo/api-gateway (45 PRs), local-demo/web-console (25 PRs), local-demo/data-pipeline (15 PRs)"
echo "Note: ClickHouse metrics_daily rollups were not populated — Dashboard trend charts backed by that table will not reflect this seed."
