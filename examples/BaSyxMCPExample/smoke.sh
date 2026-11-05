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
MCP_ENDPOINT="${MCP_ENDPOINT:-http://localhost:8081/mcp}"
# shellcheck source=smoke_lib.sh
source "${example_dir}/smoke_lib.sh"

initialized=$(mcp_request '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}')
expect_contains MCPEXAMPLE-SMOKE-INITIALIZE "${initialized}" '"name":"basyx-aas-environment"'

tools=$(mcp_request '{"jsonrpc":"2.0","id":2,"method":"tools/list"}')
expect_contains MCPEXAMPLE-SMOKE-READTOOLS "${tools}" '"name":"list_shells"'
expect_contains MCPEXAMPLE-SMOKE-WRITETOOLS "${tools}" '"name":"set_submodel_element_value"'

shells=$(mcp_tool list_shells '{"idShort":"IESEDriveMotorDM3000"}')
expect_contains MCPEXAMPLE-SMOKE-LISTSHELLS "${shells}" '"idShort":"IESEDriveMotorDM3000"'

echo "MCP example smoke test passed"
