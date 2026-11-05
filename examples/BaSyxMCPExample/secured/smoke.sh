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

set -euo pipefail

example_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MCP_ENDPOINT="${MCP_ENDPOINT:-http://localhost:8082/mcp}"
issuer="${MCP_ISSUER:-https://localhost:8443/realms/basyx-mcp}"
ca_certificate="${example_dir}/keycloak/tls/keycloak.crt"
# shellcheck source=../smoke_lib.sh
source "${example_dir}/../smoke_lib.sh"

access_token() {
  local response
  response=$(curl -sS --max-time 30 --cacert "${ca_certificate}" \
    -d grant_type=password -d client_id=basyx-mcp-test -d "username=$1" -d "password=$1" \
    -d 'scope=openid offline_access' "${issuer}/protocol/openid-connect/token")
  sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p' <<<"${response}"
}

challenge=$(curl -sS --max-time 30 -o /dev/null -D - -X POST -H 'Content-Type: application/json' -d '{}' "${MCP_ENDPOINT}")
expect_contains MCPSECURED-SMOKE-STATUS "${challenge}" ' 401'
expect_contains MCPSECURED-SMOKE-CHALLENGE "${challenge}" 'resource_metadata="http://localhost:8082/.well-known/oauth-protected-resource/mcp"'

metadata=$(curl -sS --max-time 30 http://localhost:8082/.well-known/oauth-protected-resource/mcp)
expect_contains MCPSECURED-SMOKE-METADATA "${metadata}" "\"authorization_servers\":[\"${issuer}\"]"

alice=$(access_token alice)
[ -n "${alice}" ] || fail "MCPSECURED-SMOKE-TOKEN: no access token for alice"
shells=$(mcp_tool list_shells '{"idShort":"IESEDriveMotorDM3000"}' -H "Authorization: Bearer ${alice}")
expect_contains MCPSECURED-SMOKE-READ "${shells}" '"idShort":"IESEDriveMotorDM3000"'

bob=$(access_token bob)
[ -n "${bob}" ] || fail "MCPSECURED-SMOKE-TOKEN: no access token for bob"
denied=$(mcp_tool delete_shell '{"shellId":"urn:basyx:mcp-smoke:missing"}' -H "Authorization: Bearer ${bob}")
expect_contains MCPSECURED-SMOKE-DENIED "${denied}" 'MCP-DISPATCH-HTTP403'

echo "Secured MCP example smoke test passed"
