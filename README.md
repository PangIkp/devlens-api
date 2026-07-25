# DevLens

DevLens is an Engineering Intelligence Platform for collecting GitHub workflow data and turning it into engineering metrics and insights.

This repository currently contains the initial backend foundation in [`backend`](./backend) plus local development infrastructure for PostgreSQL, ClickHouse, and NATS JetStream.

## Current Scope

- Go API bootstrap
- `/api/v1/health`
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

## Local Services

- API: `http://localhost:8080/api/v1/health`
- PostgreSQL: `localhost:5432`
- ClickHouse HTTP: `localhost:8123`
- ClickHouse Native: `localhost:9000`
- NATS: `localhost:4222`
- NATS monitoring: `http://localhost:8222`
