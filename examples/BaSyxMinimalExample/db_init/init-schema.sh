#!/bin/sh
set -eu

echo "[db-init] waiting for postgres at ${POSTGRES_HOST}:${POSTGRES_PORT}..."
until pg_isready -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; do
  sleep 2
done

echo "[db-init] postgres is ready, applying schema..."
export PGPASSWORD="${POSTGRES_PASSWORD}"
psql "host=${POSTGRES_HOST} port=${POSTGRES_PORT} user=${POSTGRES_USER} dbname=${POSTGRES_DB} sslmode=disable" \
  -v ON_ERROR_STOP=1 \
  -f /schema/basyxschema.sql

echo "[db-init] schema initialization completed."
