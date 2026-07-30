#!/usr/bin/env bash

set -euo pipefail

example_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
compose_file="${example_dir}/docker-compose.yml"
response_file="$(mktemp)"
served_infrastructure_file="$(mktemp)"
trap 'rm -f "${response_file}" "${served_infrastructure_file}"' EXIT

wait_for_url() {
  local name="$1"
  local url="$2"

  for _ in $(seq 1 60); do
    if curl --fail --silent --show-error "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for ${name} at ${url}" >&2
  return 1
}

wait_for_replicated_shell() {
  for _ in $(seq 1 60); do
    if curl --fail --silent --show-error \
      "http://127.0.0.1:8084/shells?limit=100" >"${response_file}" 2>/dev/null &&
      python3 - "${response_file}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    payload = json.load(response)

if not any(
    shell.get("idShort") == "IESEDriveMotorDM3000"
    for shell in payload.get("result", [])
):
    raise SystemExit(1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "The preconfigured AAS did not become visible through the reader" >&2
  return 1
}

query_database() {
  local service="$1"
  local query="$2"

  docker compose -f "${compose_file}" exec -T "${service}" sh -ec \
    "PGPASSWORD=\"\${POSTGRES_PASSWORD}\" psql -U \"\${POSTGRES_USER}\" -d \"\${POSTGRES_DB}\" -tAc '${query}'"
}

assert_database_routing() {
  local reader_recovery
  local reader_streaming
  local reader_connections
  local writer_connections
  local misplaced_reader_connections

  reader_recovery="$(query_database postgres-reader "SELECT pg_is_in_recovery()")"
  reader_streaming="$(query_database postgres-reader "SELECT status FROM pg_stat_wal_receiver")"
  reader_connections="$(query_database postgres-reader "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-reader\$\$")"
  writer_connections="$(query_database postgres-primary "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-writer\$\$")"
  misplaced_reader_connections="$(query_database postgres-primary "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-reader\$\$")"

  [[ "${reader_recovery}" == "t" ]]
  [[ "${reader_streaming}" == "streaming" ]]
  ((reader_connections > 0))
  ((writer_connections > 0))
  ((misplaced_reader_connections == 0))
}

assert_ui_infrastructure() {
  curl --fail --silent --show-error \
    "http://127.0.0.1:3000/config/basyx-infra.yml" >"${served_infrastructure_file}"
  if ! cmp --silent "${example_dir}/basyx-infra.yml" "${served_infrastructure_file}"; then
    echo "BaSyx Web UI does not serve the expected infrastructure configuration" >&2
    return 1
  fi
}

wait_for_url "AAS Environment" "http://127.0.0.1:8084/health"
wait_for_url "BaSyx Web UI" "http://127.0.0.1:3000"
wait_for_url "BaSyx Web UI infrastructure configuration" "http://127.0.0.1:3000/config/basyx-infra.yml"
wait_for_replicated_shell
assert_database_routing
assert_ui_infrastructure

echo "PostgreSQL read-replica example verification passed"
