/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPConfigurationDefaultsAndEnvironment(t *testing.T) {
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.False(t, cfg.MCP.Enabled)
	require.False(t, cfg.MCP.WritesEnabled)
	require.Equal(t, "/mcp", cfg.MCP.Path)
	require.Equal(t, 262144, cfg.MCP.MaxResultBytes)

	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_WRITESENABLED", "true")
	cfg, err = LoadConfig("")
	require.NoError(t, err)
	require.True(t, cfg.MCP.Enabled)
	require.True(t, cfg.MCP.WritesEnabled)
}

func TestMCPConfigurationRejectsInvalidPathsAndBudgets(t *testing.T) {
	for _, cfg := range []MCPConfig{
		{Path: "/", MaxResultBytes: 4096},
		{Path: "mcp", MaxResultBytes: 4096},
		{Path: "/mcp/", MaxResultBytes: 4096},
		{Path: "/mcp/*", MaxResultBytes: 4096},
		{Path: "/mcp", MaxResultBytes: 1},
	} {
		require.Error(t, cfg.Validate())
	}
	require.NoError(t, MCPConfig{Path: "/mcp", MaxResultBytes: 4096}.Validate())
}

func TestMCPResourceURL(t *testing.T) {
	cfg := &Config{MCP: MCPConfig{Path: "/mcp"}}
	cfg.Server.Port = 8081
	require.Equal(t, "http://localhost:8081/mcp", MCPResourceURL(cfg))

	cfg.Server.ContextPath = "/api"
	require.Equal(t, "http://localhost:8081/api/mcp", MCPResourceURL(cfg))

	cfg.General.ExternalURL = "https://basyx.example.org/api/, https://other.example.org"
	require.Equal(t, "https://basyx.example.org/api/mcp", MCPResourceURL(cfg))
}
