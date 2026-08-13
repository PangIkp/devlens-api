# DevLens

DevLens is an Engineering Intelligence Platform for collecting GitHub workflow data and turning it into engineering metrics and insights.

This repository currently contains the initial backend foundation in [`backend`](./backend) plus local development infrastructure for PostgreSQL, ClickHouse, and NATS JetStream.

## Current Scope

- Go API bootstrap
- `/api/v1/health`
- `POST /api/v1/organizations`
- `GET /api/v1/organizations`
- `GET /api/v1/organizations/{organizationId}`
- `GET /api/v1/organizations/{organizationId}/members`
- `POST /api/v1/organizations/{organizationId}/members`
- `PATCH /api/v1/organizations/{organizationId}/members/{memberId}`
- `DELETE /api/v1/organizations/{organizationId}/members/{memberId}`
- `POST /api/v1/organizations/{organizationId}/repositories`
- `GET /api/v1/organizations/{organizationId}/repositories`
- `GET /api/v1/repositories/{repositoryId}`
- `PATCH /api/v1/repositories/{repositoryId}`
- `GET /api/v1/repositories/{repositoryId}/dashboard/summary`
- `GET /api/v1/repositories/{repositoryId}/metrics/pull-requests`
- `GET /api/v1/repositories/{repositoryId}/metrics/reviews`
- `GET /api/v1/repositories/{repositoryId}/metrics/deployments`
- `GET /api/v1/repositories/{repositoryId}/metrics/hotspots`
- `POST /api/v1/repositories/{repositoryId}/sync`
- `GET /api/v1/repositories/{repositoryId}/sync-jobs`
- `GET /api/v1/sync-jobs/{syncJobId}`
- `POST /api/v1/github/webhook`
- GitHub REST client foundation for repository, pull request, review, and commit ingestion
- PostgreSQL persistence for `pull_requests` and `pull_request_reviews`
- transactional webhook delivery persistence and sync job enqueue
- ClickHouse-backed daily metrics storage for dashboard queries
- NATS JetStream trigger for repository metric recalculation after sync completion
- GitHub App connection flow for installation start, callback, repository discovery, and selection
- bearer-token session auth with login, refresh, logout, and `/api/v1/me`
- organization and repository authorization enforcement
- audit logging, trace IDs, rate limiting, and no-store cache headers
- PostgreSQL pool initialization with `pgxpool`
- SQL migrations and `sqlc` foundation
- environment-based configuration
- structured logging with `log/slog`
- chi router and core middleware
- Docker Compose for local dependencies

## Quick Start

1. Copy `.env.example` to `.env` if you want to override defaults.
2. Start local infrastructure:

```sh
docker compose up -d
```

3. Apply the PostgreSQL schema:

```sh
make migrate-up
make migrate-status
```

4. Generate SQL access code:

```sh
make sqlc-generate
```

5. Run formatting, vet, and tests through Docker:

```sh
make fmt
make vet
make test
```

6. Run the API:

```sh
make run
```

7. Run lightweight local load tests:

```sh
LOADTEST_REPOSITORY_ID=<repository-uuid> make load-dashboard
GITHUB_WEBHOOK_SECRET=<your-webhook-secret> make load-webhook
```

## Organization API

Current `Organization` shape:

- `id`
- `githubId`
- `slug`
- `name`
- `createdAt`
- `updatedAt`

Behavior notes:

- `slug` must be lowercase and may contain letters, numbers, and hyphens
- `GET` queries exclude soft-deleted organizations (`deleted_at IS NOT NULL`)
- `updatedAt` is nullable until update behavior is implemented
- organization members currently use `hard delete` because the PostgreSQL schema does not define `deleted_at` for `organization_members`
- repositories are treated as long-lived records for metrics and sync history, so this phase uses `isActive` and `archivedAt` instead of delete endpoints
- repository list supports `page`, `pageSize`, `status`, `search`, `sortBy`, and `sortOrder`
- manual sync persists pull requests and pull request reviews into PostgreSQL
- incremental sync currently uses `repositories.last_synced_at` as the cutoff
- `last_synced_at` is a coarse repository-level checkpoint and may need to evolve into finer-grained sync checkpoints in a future phase
- webhook handling validates `X-Hub-Signature-256`, stores `github_delivery_id`, and enqueues sync jobs asynchronously
- metrics are calculated from PostgreSQL raw data and stored in ClickHouse `metrics_daily`
- `deployments` and `file_changes` raw schema exist in PostgreSQL for analytics completeness, but ingestion for those sources is intentionally deferred
- hotspot ranking reads raw `file_changes` from ClickHouse
- deployment filtering by `environment` reads raw deployment analytics data from ClickHouse when available
- GitHub sync can resolve installation access through the backend GitHub App flow without exposing installation tokens to the frontend

## GitHub App Connection Flow

The backend supports GitHub App installation as the primary integration path for both public and private repositories.

Frontend-visible states:

- `not_connected`
- `installation_required`
- `connected`
- `syncing`
- `sync_failed`

Backend responsibilities:

- provide installation start and callback endpoints for GitHub App setup
- expose connection status per organization so the frontend can decide whether to show connect, install, repo selection, or sync UI
- list repositories accessible to the installation without exposing GitHub tokens to the frontend
- persist installation and repository access metadata in PostgreSQL
- trigger initial sync after the user selects repositories to connect

The endpoint contract is documented in [`docs/openapi.yaml`](./docs/openapi.yaml).

Example requests:

```sh
curl -i -X POST http://localhost:8080/api/v1/organizations \
  -H "Content-Type: application/json" \
  -d '{"githubId":123456,"slug":"devlens","name":"DevLens"}'

curl -i "http://localhost:8080/api/v1/organizations?page=1&pageSize=20"

curl -i http://localhost:8080/api/v1/organizations/{organizationId}

curl -i -X PATCH http://localhost:8080/api/v1/organizations/{organizationId} \
  -H "Content-Type: application/json" \
  -d '{"name":"DevLens Platform"}'

curl -i -X DELETE http://localhost:8080/api/v1/organizations/{organizationId}

curl -i http://localhost:8080/api/v1/organizations/{organizationId}/members

curl -i -X POST http://localhost:8080/api/v1/organizations/{organizationId}/members \
  -H "Content-Type: application/json" \
  -d '{"userId":"d18e6bc5-f4e9-4f27-8eb8-634becf5092e","role":"member"}'

curl -i -X PATCH http://localhost:8080/api/v1/organizations/{organizationId}/members/{memberId} \
  -H "Content-Type: application/json" \
  -d '{"role":"admin"}'

curl -i -X DELETE http://localhost:8080/api/v1/organizations/{organizationId}/members/{memberId}

curl -i -X POST http://localhost:8080/api/v1/organizations/{organizationId}/repositories \
  -H "Content-Type: application/json" \
  -d '{"githubId":42,"name":"devlens-api","fullName":"devlens-labs/devlens-api","defaultBranch":"main"}'

curl -i "http://localhost:8080/api/v1/organizations/{organizationId}/repositories?page=1&pageSize=20&status=active&search=devlens&sortBy=createdAt&sortOrder=desc"

curl -i http://localhost:8080/api/v1/repositories/{repositoryId}

curl -i -X PATCH http://localhost:8080/api/v1/repositories/{repositoryId} \
  -H "Content-Type: application/json" \
  -d '{"isActive":false,"archived":true}'

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/dashboard/summary?from=2026-07-01&to=2026-07-31"

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/metrics/pull-requests?from=2026-07-01&to=2026-07-31&interval=week"

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/metrics/reviews?from=2026-07-01&to=2026-07-31&interval=week"

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/metrics/deployments?from=2026-07-01&to=2026-07-31&interval=day&environment=production"

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/metrics/hotspots?from=2026-07-01&to=2026-07-31&page=1&pageSize=10&sortOrder=desc"

curl -i -X POST http://localhost:8080/api/v1/repositories/{repositoryId}/sync \
  -H "Content-Type: application/json" \
  -d '{"mode":"incremental"}'

curl -i "http://localhost:8080/api/v1/repositories/{repositoryId}/sync-jobs?page=1&pageSize=20&status=completed&sortOrder=desc"

curl -i http://localhost:8080/api/v1/sync-jobs/{syncJobId}

payload='{"action":"opened","repository":{"id":42,"full_name":"devlens-labs/devlens-api"}}'
signature=$(printf "%s" "$payload" | openssl dgst -sha256 -hmac "$GITHUB_WEBHOOK_SECRET" | sed 's/^.* //')

curl -i -X POST http://localhost:8080/api/v1/github/webhook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-GitHub-Delivery: delivery-123" \
  -H "X-Hub-Signature-256: sha256=$signature" \
  -d "$payload"
```

## GitHub Client Foundation

The current backend includes a dedicated GitHub REST client package at `backend/internal/githubclient`.

Current capabilities:

- `GetRepository`
- `ListPullRequests`
- `ListReviews`
- `ListCommits`

Implementation notes:

- uses GitHub REST API with `Authorization: Bearer <token>`
- sends `User-Agent` and `X-GitHub-Api-Version` headers on every request
- supports page-based pagination with `page` and `per_page`
- parses `Link` headers to expose the next page number
- tracks rate limit headers from GitHub responses
- retries temporary failures such as `429`, `500`, `502`, `503`, `504`, and secondary rate limit responses
- accepts a `TokenProvider` so app installation tokens and fallback token strategies can be swapped without changing sync orchestration code

New environment variables:

- `GITHUB_TOKEN`
- `GITHUB_API_BASE_URL`
- `GITHUB_USER_AGENT`
- `GITHUB_HTTP_TIMEOUT`
- `GITHUB_MAX_RETRIES`
- `GITHUB_INITIAL_BACKOFF`
- `GITHUB_MAX_BACKOFF`
- `GITHUB_WEBHOOK_SECRET`
- `SYNC_WORKER_POLL_INTERVAL`
- `CLICKHOUSE_HTTP_TIMEOUT`
- `NATS_URL`

Example:

```sh
GITHUB_TOKEN=ghp_xxx
GITHUB_API_BASE_URL=https://api.github.com
GITHUB_USER_AGENT=devlens-api
GITHUB_HTTP_TIMEOUT=10s
GITHUB_MAX_RETRIES=3
GITHUB_INITIAL_BACKOFF=500ms
GITHUB_MAX_BACKOFF=5s
GITHUB_WEBHOOK_SECRET=replace-me
SYNC_WORKER_POLL_INTERVAL=2s
CLICKHOUSE_HTTP_TIMEOUT=5s
NATS_URL=nats://nats:4222
```

Notes:

- `ListPullRequests` accepts caller-provided `state`; the Step 2 sync flow will use `state=all` by default
- GitHub App installation token support is available through the backend integration flow
- sync jobs now persist `status`, `progress`, `startedAt`, `finishedAt`, and `errorMessage` in PostgreSQL
- manual sync execution is currently inline for the current milestone
- webhook events are accepted asynchronously: the handler stores the delivery and enqueues a pending sync job, then an in-process worker processes that job
- pull request upsert uses `github_pr_id` as the primary external identity
- pull request data also enforces `(repository_id, number)` as a secondary consistency constraint
- pull request review upsert uses `github_review_id` as the provider identity
- full repository sync from webhook and direct event projection are intentionally deferred
- metrics recalculation is triggered by `repository.sync.completed` events on NATS JetStream
- deployment ingestion and file change ingestion are intentionally deferred in this branch; the schema and repositories are prepared so dashboards can safely return `0` or empty lists until those sources are populated

## Local Services

- API: `http://localhost:8080/api/v1/health`
- PostgreSQL: `localhost:5432`
- ClickHouse HTTP: `localhost:8123`
- ClickHouse Native: `localhost:9000`
- NATS: `localhost:4222`
- NATS monitoring: `http://localhost:8222`
