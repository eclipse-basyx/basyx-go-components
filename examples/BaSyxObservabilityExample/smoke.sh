#!/usr/bin/env bash

set -euo pipefail

example_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
compose_file="${example_dir}/docker-compose.yml"
trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
parent_span_id="00f067aa0ba902b7"
request_id="observability-request-1"
correlation_id="observability-correlation-1"
response_headers=$(mktemp)
response_body=$(mktemp)

cleanup() {
  rm -f "${response_headers}" "${response_body}"
}
trap cleanup EXIT

wait_for_ui() {
  local attempts=40
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl --fail --silent --output /dev/null "http://127.0.0.1:3000"; then
      return 0
    fi
    sleep 1
  done
  echo "BaSyx Web UI was not reachable" >&2
  return 1
}

wait_for_grafana_explore() {
  local attempts=40
  local status
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    status=$(curl --silent --output /dev/null --write-out "%{http_code}" \
      "http://127.0.0.1:3001/explore" || true)
    if [[ "${status}" == "200" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "Grafana Explore was not available to the anonymous user" >&2
  return 1
}

request_basyx() {
  curl --fail --silent --show-error \
    --dump-header "${response_headers}" \
    --output "${response_body}" \
    --header "traceparent: 00-${trace_id}-${parent_span_id}-01" \
    --header "X-Request-ID: ${request_id}" \
    --header "X-Correlation-ID: ${correlation_id}" \
    "http://127.0.0.1:8083/shells?limit=1"
  grep -qi "^X-Request-ID: ${request_id}" "${response_headers}"
  grep -qi "^X-Correlation-ID: ${correlation_id}" "${response_headers}"
  python3 -c '
import json
import sys

payload = json.load(sys.stdin)
shells = payload.get("result", [])
if not any(shell.get("idShort") == "IESEDriveMotorDM3000" for shell in shells):
    raise SystemExit("preconfigured IESEDriveMotorDM3000 AAS was not found")
' <"${response_body}"
}

wait_for_trace() {
  local attempts=40
  local payload
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if payload=$(curl --fail --silent "http://127.0.0.1:16686/api/traces/${trace_id}" 2>/dev/null) &&
      python3 -c 'import json,sys; data=json.load(sys.stdin).get("data", []); raise SystemExit(0 if data else 1)' <<<"${payload}"; then
      return 0
    fi
    sleep 1
  done
  echo "Trace ${trace_id} was not found in Jaeger" >&2
  return 1
}

wait_for_log() {
  local payload
  local query
  local attempts=40
  query='{service_name="aasenvironmentservice"} |= "'"${trace_id}"'" |= "'"${request_id}"'"'
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if payload=$(curl --get --fail --silent "http://127.0.0.1:3100/loki/api/v1/query_range" \
      --data-urlencode "query=${query}" \
      --data-urlencode "since=5m" \
      --data-urlencode "limit=20" 2>/dev/null) &&
      python3 -c '
import json
import sys

payload = json.load(sys.stdin)
for stream in payload.get("data", {}).get("result", []):
    for _, line in stream.get("values", []):
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if (
            record.get("msg") == "HTTP request completed"
            and record.get("trace_id") == "4bf92f3577b34da6a3ce929d0e0e4736"
            and record.get("request.id") == "observability-request-1"
            and record.get("correlation.id") == "observability-correlation-1"
            and record.get("url.path") == "/shells"
            and "limit" not in record
        ):
            raise SystemExit(0)
raise SystemExit(1)
' <<<"${payload}"; then
      return 0
    fi
    sleep 1
  done
  echo "Correlated access log was not found in Loki" >&2
  return 1
}

wait_for_ui
wait_for_grafana_explore
request_basyx
wait_for_trace
wait_for_log

if [[ "${1:-}" == "--check-collector-outage" ]]; then
  docker compose -f "${compose_file}" stop otel-collector
  request_basyx
  docker compose -f "${compose_file}" start otel-collector
fi

echo "Observability smoke verification passed"
