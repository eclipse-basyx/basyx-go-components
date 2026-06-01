#!/bin/sh
set -eu

wait_until_ready() {
  name="$1"
  url="$2"

  until curl --fail --silent --show-error "$url" >/dev/null; do
    printf 'Waiting for %s...\n' "$name"
    sleep 1
  done
}

create_resource() {
  name="$1"
  url="$2"
  payload="$3"
  response_file="/tmp/$(printf '%s' "$name" | tr ' ' '-').json"

  status="$(
    curl --silent --show-error \
      --output "$response_file" \
      --write-out '%{http_code}' \
      --request POST \
      --header 'Content-Type: application/json' \
      --data "$payload" \
      "$url"
  )"

  case "$status" in
    201|409)
      printf '%s is configured.\n' "$name"
      ;;
    *)
      printf 'Failed to create %s (HTTP %s):\n' "$name" "$status" >&2
      cat "$response_file" >&2
      exit 1
      ;;
  esac
}

configure_hydra_client() {
  client_payload='{
    "client_id": "basyx-ui",
    "client_name": "BaSyx Web UI",
    "grant_types": ["authorization_code"],
    "response_types": ["code"],
    "scope": "openid profile email",
      "redirect_uris": [
        "http://localhost:3000",
        "http://localhost:3000/",
        "http://localhost:3000/aasviewer",
        "http://localhost:3000/aaseditor"
      ],
      "post_logout_redirect_uris": [
        "http://localhost:3000/aasviewer",
        "http://localhost:3000/aaseditor"
      ],
      "audience": ["basyx-api"],
      "token_endpoint_auth_method": "none"
    }'

  create_resource "Hydra BaSyx UI client" "$HYDRA_ADMIN_URL/admin/clients" "$client_payload"

  status="$(
    curl --silent --show-error \
      --output /tmp/hydra-basyx-ui-client.json \
      --write-out '%{http_code}' \
      --request PUT \
      --header 'Content-Type: application/json' \
      --data "$client_payload" \
      "$HYDRA_ADMIN_URL/admin/clients/basyx-ui"
  )"

  if [ "$status" != 200 ]; then
    printf 'Failed to update Hydra BaSyx UI client (HTTP %s):\n' "$status" >&2
    cat /tmp/hydra-basyx-ui-client.json >&2
    exit 1
  fi
}

wait_until_ready "Hydra" "$HYDRA_ADMIN_URL/health/ready"
wait_until_ready "Kratos" "$KRATOS_ADMIN_URL/health/ready"

configure_hydra_client

create_resource "Kratos admin identity" "$KRATOS_ADMIN_URL/admin/identities" '{
  "schema_id": "default",
  "traits": {
    "email": "admin@example.com",
    "role": "admin"
  },
  "credentials": {
    "password": {
      "config": {
        "password": "pwd"
      }
    }
  }
}'

create_resource "Kratos viewer identity" "$KRATOS_ADMIN_URL/admin/identities" '{
  "schema_id": "default",
  "traits": {
    "email": "viewer@example.com",
    "role": "viewer"
  },
  "credentials": {
    "password": {
      "config": {
        "password": "pwd"
      }
    }
  }
}'
