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

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <backup-file>" >&2
  exit 2
fi

BACKUP_FILE="$1"
if [[ ! -f "${BACKUP_FILE}" ]]; then
  echo "backup file not found: ${BACKUP_FILE}" >&2
  exit 1
fi

echo "restoring postgres backup: ${BACKUP_FILE}"

docker compose -f "${ROOT_DIR}/docker-compose.yml" exec -T postgres \
  pg_restore \
  --username="${POSTGRES_USER}" \
  --dbname="${POSTGRES_DB}" \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  < "${BACKUP_FILE}"

echo "restore completed from: ${BACKUP_FILE}"
