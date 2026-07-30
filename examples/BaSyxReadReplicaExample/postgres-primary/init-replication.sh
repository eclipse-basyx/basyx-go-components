#!/usr/bin/env bash

set -euo pipefail

psql \
  --set=ON_ERROR_STOP=1 \
  --set=replication_user="${REPLICATION_USER}" \
  --set=replication_password="${REPLICATION_PASSWORD}" \
  --username "${POSTGRES_USER}" \
  --dbname "${POSTGRES_DB}" <<'SQL'
SELECT format(
  'CREATE ROLE %I WITH REPLICATION LOGIN PASSWORD %L',
  :'replication_user',
  :'replication_password'
)
\gexec
SQL

printf '\nhost replication %s samenet scram-sha-256\n' "${REPLICATION_USER}" >>"${PGDATA}/pg_hba.conf"
