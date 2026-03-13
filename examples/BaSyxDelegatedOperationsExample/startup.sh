#!/bin/sh

set -eu

wait_for_service() {
  service_name="$1"
  service_url="$2"

  echo "Waiting for ${service_name} to be ready..."
  attempt=0
  max_attempts=60

  while [ "$attempt" -lt "$max_attempts" ]; do
    if curl -s -f "$service_url" >/dev/null 2>&1; then
      echo "${service_name} is healthy"
      return 0
    fi

    attempt=$((attempt + 1))
    sleep 2
  done

  echo "DATAINIT-WAITSERVICE-TIMEOUT ${service_name} did not become ready in time"
  exit 1
}

post_json_if_missing() {
  get_url="$1"
  post_url="$2"
  payload_file="$3"
  object_name="$4"

  get_status="$(curl -s -o /dev/null -w "%{http_code}" "$get_url")"
  if [ "$get_status" = "200" ]; then
    echo "${object_name} already exists, skipping"
    return 0
  fi

  if [ "$get_status" != "404" ]; then
    echo "DATAINIT-CHECK-${object_name} unexpected status ${get_status} while checking existing object"
    exit 1
  fi

  post_status="$(curl -s -o /dev/null -w "%{http_code}" -X POST "$post_url" -H "Content-Type: application/json" --data @"$payload_file")"
  if [ "$post_status" != "201" ]; then
    echo "DATAINIT-POST-${object_name} failed with status ${post_status}"
    exit 1
  fi

  echo "${object_name} created"
}

wait_for_service "AAS Repository" "http://aas_repository:5004/health"
wait_for_service "Submodel Repository" "http://submodel_repository:5004/health"

encoded_submodel_id="aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvc20vZGVsZWdhdGVkLW9wZXJhdGlvbnM"
encoded_aas_id="aHR0cHM6Ly9leGFtcGxlLmNvbS9pZHMvYWFzL2RlbGVnYXRlZC1vcGVyYXRpb25zLWV4YW1wbGU"

post_json_if_missing \
  "http://submodel_repository:5004/submodels/${encoded_submodel_id}" \
  "http://submodel_repository:5004/submodels" \
  "/data/submodel-delegated-operations.json" \
  "submodel"

post_json_if_missing \
  "http://aas_repository:5004/shells/${encoded_aas_id}" \
  "http://aas_repository:5004/shells" \
  "/data/aas-shell.json" \
  "aas"

echo "Delegated operations example data initialized"
