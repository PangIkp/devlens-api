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
BACKUP_DIR="${BACKUP_DIR:-${ROOT_DIR}/backups/postgres}"
TIMESTAMP="$(date -u +"%Y%m%dT%H%M%SZ")"
BACKUP_FILE="${1:-${BACKUP_DIR}/${POSTGRES_DB}-${TIMESTAMP}.dump}"

mkdir -p "$(dirname "${BACKUP_FILE}")"

echo "creating postgres backup: ${BACKUP_FILE}"

docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
  pg_dump \
  --username="${POSTGRES_USER}" \
  --dbname="${POSTGRES_DB}" \
  --format=custom \
  --no-owner \
  --no-privileges \
  > "${BACKUP_FILE}"

echo "backup completed: ${BACKUP_FILE}"
