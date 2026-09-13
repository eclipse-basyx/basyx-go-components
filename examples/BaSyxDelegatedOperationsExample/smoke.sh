#!/usr/bin/env bash

set -euo pipefail

example_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
full_request_file="${example_dir}/data/invoke-request-add-5-and-3.json"
value_only_request_file="${example_dir}/data/invoke-request-add-5-and-3-value-only.json"
aas_environment_url="http://localhost:8090"
delegated_service_url="http://localhost:8099"
encoded_submodel_id="aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM"
encoded_aas_id="aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzL2RlbGVnYXRlZC1vcGVyYXRpb25zLWV4YW1wbGU"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT

wait_for_url() {
  local name="$1"
  local url="$2"

  for _ in $(seq 1 60); do
    if curl --fail --silent --show-error "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for ${name} at ${url}" >&2
  return 1
}

assert_completed_sum() {
  local response_file="$1"
  local representation="$2"

  python3 - "$response_file" "$representation" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    payload = json.load(response)

if payload.get("executionState") != "Completed":
    raise SystemExit(f"unexpected executionState: {payload.get('executionState')!r}")
if payload.get("success") is not True:
    raise SystemExit(f"unexpected success value: {payload.get('success')!r}")

if sys.argv[2] == "value-only":
    if payload.get("outputArguments", {}).get("sum") != "8":
        raise SystemExit("completed value-only result does not contain output argument sum=8")
    raise SystemExit(0)

for argument in payload.get("outputArguments", []):
    value = argument.get("value", {})
    if value.get("idShort") == "sum" and value.get("value") == "8":
        break
else:
    raise SystemExit("completed result does not contain output argument sum=8")
PY
}

resolve_location() {
  python3 - "$1" "$2" <<'PY'
import sys
from urllib.parse import urljoin

print(urljoin(sys.argv[1], sys.argv[2]))
PY
}

read_location() {
  awk 'tolower($1) == "location:" {sub(/^[^:]*:[[:space:]]*/, ""); print}' "$1" |
    tr -d '\r' |
    tail -n 1
}

invoke_async() {
  local invoke_url="$1"
  local invocation_name="$2"
  local request_file="$3"
  local representation="$4"
  local response_body="${temporary_dir}/${invocation_name}-body.json"
  local response_headers="${temporary_dir}/${invocation_name}-headers.txt"
  local status_code
  local location

  status_code="$(
    curl --silent --show-error \
      --output "$response_body" \
      --dump-header "$response_headers" \
      --write-out '%{http_code}' \
      --request POST \
      --header 'Content-Type: application/json' \
      --data @"$request_file" \
      "$invoke_url"
  )"
  if [ "$status_code" != "202" ]; then
    echo "${invocation_name} invocation returned ${status_code}" >&2
    cat "$response_body" >&2
    return 1
  fi

  location="$(read_location "$response_headers")"
  if [ -z "$location" ]; then
    echo "${invocation_name} invocation did not return a Location header" >&2
    return 1
  fi
  location="$(resolve_location "$invoke_url" "$location")"

  for _ in $(seq 1 30); do
    status_code="$(
      curl --silent --show-error \
        --output "$response_body" \
        --dump-header "$response_headers" \
        --write-out '%{http_code}' \
        "$location"
    )"

    if [ "$status_code" = "200" ]; then
      python3 - "$response_body" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    payload = json.load(response)
if payload.get("executionState") not in {"Initiated", "Running"}:
    raise SystemExit(f"unexpected asynchronous status: {payload!r}")
PY
      sleep 1
      continue
    fi

    if [ "$status_code" != "302" ]; then
      echo "${invocation_name} status endpoint returned ${status_code}" >&2
      cat "$response_body" >&2
      return 1
    fi

    local result_location
    result_location="$(read_location "$response_headers")"
    if [ -z "$result_location" ]; then
      echo "${invocation_name} status endpoint did not return a Location header" >&2
      return 1
    fi
    result_location="$(resolve_location "$location" "$result_location")"
    if [ "$representation" = "value-only" ]; then
      result_location="${result_location}/\$value"
    fi

    status_code="$(
      curl --silent --show-error \
        --output "$response_body" \
        --write-out '%{http_code}' \
        "$result_location"
    )"
    if [ "$status_code" != "200" ]; then
      echo "${invocation_name} result endpoint returned ${status_code}" >&2
      cat "$response_body" >&2
      return 1
    fi

    assert_completed_sum "$response_body" "$representation"
    return 0
  done

  echo "${invocation_name} did not complete in time" >&2
  return 1
}

invoke_sync() {
  local invoke_url="$1"
  local invocation_name="$2"
  local request_file="$3"
  local representation="$4"
  local response_body="${temporary_dir}/${invocation_name}-body.json"
  local status_code

  status_code="$(
    curl --silent --show-error \
      --output "$response_body" \
      --write-out '%{http_code}' \
      --request POST \
      --header 'Content-Type: application/json' \
      --data @"$request_file" \
      "$invoke_url"
  )"
  if [ "$status_code" != "200" ]; then
    echo "${invocation_name} invocation returned ${status_code}" >&2
    cat "$response_body" >&2
    return 1
  fi
  assert_completed_sum "$response_body" "$representation"
}

wait_for_url "AAS Environment" "${aas_environment_url}/health"
wait_for_url "delegated operation service" "${delegated_service_url}/health"
wait_for_url "delegated operations example data" "${aas_environment_url}/submodels/${encoded_submodel_id}"

for repository_path in \
  "submodels/${encoded_submodel_id}" \
  "shells/${encoded_aas_id}/submodels/${encoded_submodel_id}"; do
  repository_name="$(printf '%s' "$repository_path" | tr '/:' '--')"

  invoke_sync \
    "${aas_environment_url}/${repository_path}/submodel-elements/AddNumbersSync/invoke" \
    "${repository_name}-sync-full" \
    "$full_request_file" \
    "full"
  invoke_async \
    "${aas_environment_url}/${repository_path}/submodel-elements/AddNumbersAsync/invoke-async" \
    "${repository_name}-async-full" \
    "$full_request_file" \
    "full"
  invoke_sync \
    "${aas_environment_url}/${repository_path}/submodel-elements/AddNumbersSyncValueOnly/invoke/\$value" \
    "${repository_name}-sync-value-only" \
    "$value_only_request_file" \
    "value-only"
  invoke_async \
    "${aas_environment_url}/${repository_path}/submodel-elements/AddNumbersAsyncValueOnly/invoke-async/\$value" \
    "${repository_name}-async-value-only" \
    "$value_only_request_file" \
    "value-only"
done

echo "Delegated operations protocol smoke test passed"
