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
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// MCPConfig controls the optional MCP endpoint of the AAS Environment.
type MCPConfig struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Path           string `mapstructure:"path" yaml:"path" json:"path"`
	WritesEnabled  bool   `mapstructure:"writesEnabled" yaml:"writesEnabled" json:"writesEnabled"`
	MaxResultBytes int    `mapstructure:"maxResultBytes" yaml:"maxResultBytes" json:"maxResultBytes"`
}

const minMCPResultBytes = 1024

func setMCPDefaults(v *viper.Viper) {
	v.SetDefault("mcp.enabled", false)
	v.SetDefault("mcp.path", "/mcp")
	v.SetDefault("mcp.writesEnabled", false)
	v.SetDefault("mcp.maxResultBytes", 262144)
}

// Validate rejects MCP paths that cannot be mounted beside the API and unusable result budgets.
func (c MCPConfig) Validate() error {
	if c.Path == "/" || !strings.HasPrefix(c.Path, "/") || strings.HasSuffix(c.Path, "/") || strings.ContainsAny(c.Path, "?#%{}*") || strings.Contains(c.Path, "..") {
		return fmt.Errorf("MCP-VALIDATECONFIG-PATH: use a non-root absolute path without a trailing slash")
	}
	if c.MaxResultBytes < minMCPResultBytes {
		return fmt.Errorf("MCP-VALIDATECONFIG-MAXRESULTBYTES: maxResultBytes must be at least %d", minMCPResultBytes)
	}
	return nil
}

// MCPResourceURL returns the public URL of the MCP endpoint used as the OAuth protected resource identifier.
func MCPResourceURL(cfg *Config) string {
	return strings.TrimRight(PublicBaseURL(cfg), "/") + cfg.MCP.Path
}
