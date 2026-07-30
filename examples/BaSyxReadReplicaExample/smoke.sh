#!/usr/bin/env bash
#
# Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
#
# Permission is hereby granted, free of charge, to any person obtaining
# a copy of this software and associated documentation files (the
# "Software"), to deal in the Software without restriction, including
# without limitation the rights to use, copy, modify, merge, publish,
# distribute, sublicense, and/or sell copies of the Software, and to
# permit persons to whom the Software is furnished to do so, subject to
# the following conditions:
#
# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
# LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
# OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
# WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
#
# SPDX-License-Identifier: MIT

set -euo pipefail

example_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
compose_file="${example_dir}/docker-compose.yml"
response_file="$(mktemp)"
trap 'rm -f "${response_file}"' EXIT

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
  local reader_connections
  local writer_connections
  local misplaced_reader_connections

  reader_recovery="$(query_database postgres-reader "SELECT pg_is_in_recovery()")"
  reader_connections="$(query_database postgres-reader "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-reader\$\$")"
  writer_connections="$(query_database postgres-primary "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-writer\$\$")"
  misplaced_reader_connections="$(query_database postgres-primary "SELECT count(*) FROM pg_stat_activity WHERE application_name = \$\$aasenvironmentservice-reader\$\$")"

  [[ "${reader_recovery}" == "t" ]]
  ((reader_connections > 0))
  ((writer_connections > 0))
  ((misplaced_reader_connections == 0))
}

wait_for_url "AAS Environment" "http://127.0.0.1:8084/health"
wait_for_url "BaSyx Web UI" "http://127.0.0.1:3000"
wait_for_replicated_shell
assert_database_routing

echo "PostgreSQL read-replica example verification passed"
