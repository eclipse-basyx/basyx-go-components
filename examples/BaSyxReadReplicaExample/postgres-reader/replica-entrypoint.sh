#!/usr/bin/env bash

set -euo pipefail

basebackup_marker="${PGDATA}/.basyx-basebackup-complete"
replication_slot="${REPLICATION_SLOT:-basyx_reader}"
force_clone=false

if [[ ! "${replication_slot}" =~ ^[a-z0-9_]+$ ]]; then
  echo "REPLICATION_SLOT must contain only lowercase letters, numbers, and underscores" >&2
  exit 1
fi

prepare_data_directory() {
  mkdir -p "${PGDATA}"
  chown -R postgres:postgres "${PGDATA}"
  find "${PGDATA}" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
}

query_primary() {
  PGPASSWORD="${POSTGRES_PASSWORD}" gosu postgres psql \
    --host="${PRIMARY_HOST}" \
    --port="${PRIMARY_PORT}" \
    --username="${POSTGRES_USER}" \
    --dbname="${POSTGRES_DB}" \
    --tuples-only \
    --no-align \
    --set=ON_ERROR_STOP=1 \
    --command="$1"
}

prepare_replication_slot() {
  local slot_state
  slot_state="$(query_primary "SELECT COALESCE(invalidation_reason, 'valid') FROM pg_replication_slots WHERE slot_name = '${replication_slot}' AND slot_type = 'physical'")"

  if [[ -z "${slot_state}" ]]; then
    query_primary "SELECT pg_create_physical_replication_slot('${replication_slot}', true)" >/dev/null
    if [[ -f "${basebackup_marker}" ]]; then
      force_clone=true
    fi
    return
  fi
  if [[ "${slot_state}" == "valid" ]]; then
    return
  fi

  echo "Replication slot ${replication_slot} was invalidated (${slot_state}); recreating the standby" >&2
  query_primary "SELECT pg_drop_replication_slot('${replication_slot}')" >/dev/null
  query_primary "SELECT pg_create_physical_replication_slot('${replication_slot}', true)" >/dev/null
  force_clone=true
}

clone_primary() {
  local connection_string
  connection_string="host=${PRIMARY_HOST} port=${PRIMARY_PORT} user=${REPLICATION_USER} password=${REPLICATION_PASSWORD}"

  for attempt in $(seq 1 30); do
    prepare_data_directory
    if gosu postgres pg_basebackup \
      --dbname="${connection_string}" \
      --pgdata="${PGDATA}" \
      --checkpoint=fast \
      --wal-method=stream \
      --slot="${replication_slot}" \
      --write-recovery-conf; then
      touch "${basebackup_marker}"
      chmod 0700 "${PGDATA}"
      return 0
    fi
    echo "Primary clone attempt ${attempt} failed; retrying" >&2
    sleep 2
  done

  echo "Unable to clone PostgreSQL primary" >&2
  return 1
}

prepare_replication_slot
if [[ "${force_clone}" == "true" || ! -s "${PGDATA}/PG_VERSION" || ! -f "${basebackup_marker}" ]]; then
  clone_primary
fi

exec docker-entrypoint.sh postgres -c hot_standby=on
