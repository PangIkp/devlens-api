# Operations Runbook

This runbook covers the minimum operational procedures currently implemented in the DevLens backend repository.

## Backup Configuration

PostgreSQL backup and restore are file-based and run against the local Docker Compose `postgres` service.

Environment variables:

- `POSTGRES_USER`
- `POSTGRES_DB`
- `BACKUP_DIR`
- `BACKUP_FILE`
- `COMPOSE_PROJECT_NAME`

Default behavior:

- `make backup-postgres` writes a custom-format dump to `backups/postgres/<database>-<timestamp>.dump`
- `make restore-postgres` restores a selected dump into the configured database
- `make verify-postgres-restore` restores a selected dump into a temporary verification database and checks core tables

Recommended local flow:

```sh
docker compose up -d postgres
make backup-postgres
BACKUP_FILE=/absolute/path/to/backup.dump make verify-postgres-restore
```

## Rollback Plan

Current rollback support in this repository is based on traceable build metadata plus database backup/restore procedures.

Application rollback:

1. Identify the previously known-good backend image or commit SHA.
2. Redeploy that exact image tag or rebuild from that commit.
3. Confirm the running build via startup logs:
   - `version`
   - `commit`
   - `build_time`
4. Verify `GET /api/v1/health` returns dependency status `ok`.

Database rollback:

1. Stop write traffic to the API.
2. Take an additional fresh backup before rollback.
3. Restore the target PostgreSQL backup:

```sh
BACKUP_FILE=/absolute/path/to/backup.dump make restore-postgres
```

4. Run migration status verification:

```sh
make migrate-status
```

5. Restart the API and re-check health and key read paths.

## Restore Verification

`make verify-postgres-restore` is a non-primary restore safety check. It creates a temporary database, restores the chosen backup there, validates that key tables exist, and then deletes the temporary database.

Example:

```sh
BACKUP_FILE=/absolute/path/to/backup.dump make verify-postgres-restore
```

Verified tables:

- `schema_migrations`
- `organizations`
- `repositories`
- `sync_jobs`
- `webhook_deliveries`
- `insight_statuses`

## Observability

The local observability stack includes:

- Prometheus scraping `http://host.docker.internal:8080/metrics`
- Grafana with provisioned dashboards for backend overview, workers, GitHub API, and sync/webhook operations
- baseline alert rules for HTTP 5xx ratio, latency, queue lag, GitHub rate limit budget, sync failures, and webhook delay

Tracing support:

- enable OTLP export with `OTEL_ENABLED=true`
- set `OTEL_EXPORTER_OTLP_ENDPOINT=<host:port>`
- optional controls:
  - `OTEL_SERVICE_NAME`
  - `OTEL_EXPORTER_OTLP_INSECURE`
  - `OTEL_TRACE_SAMPLE_RATIO`

## Current Gaps

This runbook does not yet automate:

- remote object storage for backups
- scheduled backups
- ClickHouse backup and restore
- NATS state backup
- production deployment orchestration
