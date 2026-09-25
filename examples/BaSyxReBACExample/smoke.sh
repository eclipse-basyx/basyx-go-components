#!/usr/bin/env bash
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

# Verifies creator ownership, sharing and revocation with ReBAC, including
# the synchronized registry descriptor that follows its Submodel.
set -euo pipefail

base_url="${BASYX_REBAC_EXAMPLE_URL:-http://localhost:8082}"
token_url="${BASYX_REBAC_TOKEN_URL:-http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token}"
issuer="${token_url%/protocol/openid-connect/token}"

token() {
  curl -sf -d "grant_type=password&client_id=basyx-ui&username=$1&password=pwd" "$token_url" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])'
}

subject() {
  python3 -c 'import base64,json,sys; p=sys.argv[1].split(".")[1]; print(json.loads(base64.urlsafe_b64decode(p+"="*(-len(p)%4)))["sub"])' "$1"
}

status() {
  curl -s -o /dev/null -w '%{http_code}' "$@"
}

expect() {
  if [ "$2" != "$1" ]; then
    echo "REBAC-SMOKE-$3: expected $1, got $2" >&2
    exit 1
  fi
}

etag() {
  curl -sf -D - -o /dev/null -H "Authorization: Bearer $1" "$2" | tr -d '\r' | awk -F': ' 'tolower($1)=="etag"{print $2}'
}

for attempt in $(seq 1 60); do
  if [ "$(status "$base_url/health")" = "200" ]; then
    break
  fi
  sleep 5
done

dave="$(token dave)"
alice="$(token alice)"
bob="$(token bob)"
alice_sub="$(subject "$alice")"
bob_sub="$(subject "$bob")"

grant() {
  printf '{"relation":"%s","subjectType":"user","issuer":"%s","subject":"%s"}' "$1" "$issuer" "$2"
}

put_grants() {
  local caller="$1" access="$2" grants="$3" match
  match="$(etag "$caller" "$access")"
  status -X PUT -H "Authorization: Bearer $caller" -H "If-Match: $match" -H 'Content-Type: application/json' \
    --data-raw "{\"grants\":[$grants]}" "$access/grants"
}

repository="$base_url/security/rebac/repositories/submodel/\$access"
expect 200 "$(put_grants "$dave" "$repository" "$(grant creator "$alice_sub")")" BOOTSTRAP

submodel_id="urn:basyx:rebac-example:$(date +%s)"
encoded_id="$(printf '%s' "$submodel_id" | base64 | tr '+/' '-_' | tr -d '=\n')"
submodel_body="$(printf '{"id":"%s","idShort":"shared","modelType":"Submodel"}' "$submodel_id")"
expect 201 "$(status -X POST -H "Authorization: Bearer $alice" -H 'Content-Type: application/json' \
  --data-raw "$submodel_body" "$base_url/submodels")" CREATE
expect 403 "$(status -H "Authorization: Bearer $bob" "$base_url/submodels/$encoded_id")" UNSHARED

access="$base_url/submodels/$encoded_id/\$access"
owner="$(grant owner "$alice_sub")"
descriptor="$base_url/submodel-descriptors/$encoded_id"
expect 403 "$(status -H "Authorization: Bearer $bob" "$descriptor")" UNSHARED-DESCRIPTOR

expect 200 "$(put_grants "$alice" "$access" "$owner,$(grant viewer "$bob_sub")")" SHARE
expect 200 "$(status -H "Authorization: Bearer $bob" "$base_url/submodels/$encoded_id")" SHARED
expect 200 "$(status -H "Authorization: Bearer $bob" "$descriptor")" SHARED-DESCRIPTOR

expect 200 "$(put_grants "$alice" "$access" "$owner")" REVOKE
expect 403 "$(status -H "Authorization: Bearer $bob" "$base_url/submodels/$encoded_id")" REVOKED

echo "ReBAC example smoke test passed"
