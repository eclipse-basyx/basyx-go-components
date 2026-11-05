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

# Helpers for the MCP example smoke tests. Source this file; it expects MCP_ENDPOINT to be set.

fail() {
  echo "$1" >&2
  exit 1
}

# mcp_request <json-rpc body> [curl options...] prints the MCP response body.
mcp_request() {
  local body="$1"
  shift
  curl -sS --max-time 30 \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -H 'MCP-Protocol-Version: 2025-06-18' \
    "$@" -d "$body" "${MCP_ENDPOINT}"
}

# mcp_tool <tool name> <arguments json> [curl options...] calls one tool.
mcp_tool() {
  local name="$1" arguments="$2"
  shift 2
  mcp_request "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"${name}\",\"arguments\":${arguments}}}" "$@"
}

# expect_contains <code> <text> <expected> fails with code when text lacks expected.
expect_contains() {
  if ! grep -qF -- "$3" <<<"$2"; then
    fail "$1: expected '$3' in: ${2:0:500}"
  fi
}
