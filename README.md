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
```

## Local Services

- API: `http://localhost:8080/api/v1/health`
- PostgreSQL: `localhost:5432`
- ClickHouse HTTP: `localhost:8123`
- ClickHouse Native: `localhost:9000`
- NATS: `localhost:4222`
- NATS monitoring: `http://localhost:8222`
