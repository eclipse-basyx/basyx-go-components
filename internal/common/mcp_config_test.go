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
	require.Equal(t, "disabled", cfg.MCP.WriteMode)
	require.Equal(t, 65536, cfg.MCP.Limits.MaxResultBytes)
	t.Setenv("BASYX_MCP_ENABLED", "true")
	cfg, err = LoadConfig("")
	require.NoError(t, err)
	require.True(t, cfg.MCP.Enabled)
}

func TestMCPConfigurationRejectsUnsafeLimitsAndPaths(t *testing.T) {
	for _, cfg := range []MCPConfig{
		{Path: "/", WriteMode: "disabled", Limits: MCPLimits{MaxResultBytes: 1}},
		{Path: "/mcp", WriteMode: "automatic"},
	} {
		require.Error(t, cfg.Validate())
	}
}
