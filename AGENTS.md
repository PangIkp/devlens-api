# DevLens Agent Guide

## Project Overview

DevLens is an Engineering Intelligence Platform. It ingests engineering workflow data from GitHub, stores transactional state in PostgreSQL, stores analytical/event data in ClickHouse, and exposes API endpoints for dashboards, metrics, repository health, and operational sync workflows.

This repository currently contains the initial backend foundation only. Follow the OpenAPI contract in `docs/openapi.yaml` as the source of truth for API behavior.

## Confirmed Technology Stack

- Go
- chi router
- pgx
- sqlc
- PostgreSQL
- ClickHouse
- NATS JetStream
- Docker Compose
- OpenTelemetry
- Prometheus
- Grafana
- GitHub Actions

## Architecture Rules

- Keep transport, configuration, infrastructure, and domain concerns separate.
- Prefer explicit dependency injection in constructors and top-level wiring.
- Avoid unnecessary interfaces until multiple implementations are required.
- Treat `docs/openapi.yaml` as the API contract when design docs conflict.
- Build the HTTP API as `/api/v1`.
- Keep webhook handling, ingestion, metric calculation, and storage concerns isolated when those features are added.

## Folder Structure Rules

- `backend/cmd/*` contains executable entrypoints only.
- `backend/internal/config` owns environment parsing and validation.
- `backend/internal/app` owns application bootstrap and lifecycle wiring.
- `backend/internal/httpapi` owns router construction, handlers, middleware, and API response helpers.
- Add future storage code under focused packages such as `internal/postgres`, `internal/clickhouse`, and `internal/nats`.
- Keep tests close to the package they validate unless an end-to-end test requires a higher-level package.

## Dependency Direction

- `cmd` depends on `internal/app`.
- `internal/app` may depend on config, logging, and HTTP transport packages.
- `internal/httpapi` may depend on simple domain/service dependencies injected into handlers.
- Config must not depend on transport or infrastructure packages.
- Future repository packages must not import HTTP packages.

## API Conventions

- Use `/api/v1` URL versioning.
- Use plural resource names.
- Use JSON for all API responses.
- Use `{ "data": ... }` for standard success payloads unless the OpenAPI contract explicitly defines a different shape.
- Use `{ "error": { "code": "...", "message": "...", "requestId": "..." } }` for errors.
- Return validation errors with `details` when field-level issues exist.

## Error-Handling Conventions

- Use explicit HTTP status codes.
- Prefer stable machine-readable error codes in `SCREAMING_SNAKE_CASE`.
- Include request IDs in error payloads when available.
- Recover panics at the HTTP boundary and return a generic internal error response.
- Do not leak internal implementation details in client-facing errors.

## Logging Conventions

- Use Go standard library `log/slog`.
- Emit structured logs only.
- Include `request_id`, `method`, `path`, `status`, and `duration_ms` for HTTP access logs.
- Never log secrets, tokens, or raw credentials.

## Database Conventions

- PostgreSQL is the transactional system of record for users, organizations, installations, repository configuration, sync state, and operational metadata.
- ClickHouse stores analytics-oriented events, denormalized metrics, and dashboard aggregates.
- Use `pgx` and `sqlc` for PostgreSQL access.
- Do not introduce an ORM.
- Keep SQL explicit and versioned when migrations are added later.

## Testing Conventions

- Use Go's standard `testing` package by default.
- Prefer table-driven tests for handler behavior.
- Keep unit tests fast and deterministic.
- Use integration tests only when behavior spans real dependencies and the added value is clear.

## Rules For Future Coding Agents

- Read the required docs before major changes.
- Do not expand the architecture beyond the current phase without an explicit request.
- Do not add database connections, migrations, GitHub integration, workers, or metrics logic unless requested.
- Preserve the API contract and document any intentional deviation before changing behavior.
- Keep the code maintainable for a solo developer.
