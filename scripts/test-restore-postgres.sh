#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi

POSTGRES_USER="${POSTGRES_USER:-devlens}"
POSTGRES_DB="${POSTGRES_DB:-devlens}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-devlens}"
VERIFY_DB="${VERIFY_DB:-${POSTGRES_DB}_restore_verify_$(date -u +%Y%m%d%H%M%S)}"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <backup-file>" >&2
  exit 2
fi

BACKUP_FILE="$1"
if [[ ! -f "${BACKUP_FILE}" ]]; then
  echo "backup file not found: ${BACKUP_FILE}" >&2
  exit 1
fi

cleanup() {
  docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
    psql \
    --username="${POSTGRES_USER}" \
    --dbname=postgres \
    --command="DROP DATABASE IF EXISTS \"${VERIFY_DB}\" WITH (FORCE);" \
    >/dev/null
}

trap cleanup EXIT

echo "creating temporary restore verification database: ${VERIFY_DB}"

docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
  psql \
  --username="${POSTGRES_USER}" \
  --dbname=postgres \
  --command="DROP DATABASE IF EXISTS \"${VERIFY_DB}\" WITH (FORCE);" \
  >/dev/null

docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
  psql \
  --username="${POSTGRES_USER}" \
  --dbname=postgres \
  --command="CREATE DATABASE \"${VERIFY_DB}\";" \
  >/dev/null

echo "restoring backup into temporary database: ${VERIFY_DB}"

docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
  pg_restore \
  --username="${POSTGRES_USER}" \
  --dbname="${VERIFY_DB}" \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  < "${BACKUP_FILE}"

echo "verifying restored schema and data access"

tables_restored="$(
  docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
    psql \
    --username="${POSTGRES_USER}" \
    --dbname="${VERIFY_DB}" \
    --tuples-only \
    --no-align \
    --command="SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';"
)"

if [[ "${tables_restored}" -lt 5 ]]; then
  echo "restore verification failed: expected at least 5 public tables, got ${tables_restored}" >&2
  exit 1
fi

for table_name in schema_migrations organizations repositories sync_jobs webhook_deliveries insight_statuses; do
  docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
    psql \
    --username="${POSTGRES_USER}" \
    --dbname="${VERIFY_DB}" \
    --tuples-only \
    --no-align \
    --command="SELECT to_regclass('public.${table_name}') IS NOT NULL;" \
    | grep -qx "t" || {
      echo "restore verification failed: missing table ${table_name}" >&2
      exit 1
    }
done

echo "restore verification succeeded for backup: ${BACKUP_FILE}"
echo "postgres service: ${COMPOSE_PROJECT_NAME} (${POSTGRES_HOST}:${POSTGRES_PORT})"
