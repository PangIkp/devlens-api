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
- GitHub REST client foundation for repository, pull request, review, and commit ingestion
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
- authentication is still deferred, so member write endpoints currently accept `userId` in request body
- organization members currently use `hard delete` because the PostgreSQL schema does not define `deleted_at` for `organization_members`
- repositories are treated as long-lived records for metrics and sync history, so this phase uses `isActive` and `archivedAt` instead of delete endpoints
- repository list supports `page`, `pageSize`, `status`, `search`, `sortBy`, and `sortOrder`

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
- accepts a `TokenProvider` so future GitHub App installation tokens can be introduced without changing sync orchestration code

New environment variables:

- `GITHUB_TOKEN`
- `GITHUB_API_BASE_URL`
- `GITHUB_USER_AGENT`
- `GITHUB_HTTP_TIMEOUT`
- `GITHUB_MAX_RETRIES`
- `GITHUB_INITIAL_BACKOFF`
- `GITHUB_MAX_BACKOFF`

Example:

```sh
GITHUB_TOKEN=ghp_xxx
GITHUB_API_BASE_URL=https://api.github.com
GITHUB_USER_AGENT=devlens-api
GITHUB_HTTP_TIMEOUT=10s
GITHUB_MAX_RETRIES=3
GITHUB_INITIAL_BACKOFF=500ms
GITHUB_MAX_BACKOFF=5s
```

Notes:

- `ListPullRequests` accepts caller-provided `state`; the Step 2 sync flow will use `state=all` by default
- GitHub App installation token support is intentionally deferred in this phase
- This step does not add sync jobs, pull request persistence, or webhook endpoints yet

## Local Services

- API: `http://localhost:8080/api/v1/health`
- PostgreSQL: `localhost:5432`
- ClickHouse HTTP: `localhost:8123`
- ClickHouse Native: `localhost:9000`
- NATS: `localhost:4222`
- NATS monitoring: `http://localhost:8222`
