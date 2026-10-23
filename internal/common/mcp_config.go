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
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// MCPConfig controls the optional integrated agent interface.
type MCPConfig struct {
	Enabled              bool                  `mapstructure:"enabled" yaml:"enabled"`
	Path                 string                `mapstructure:"path" yaml:"path"`
	AllowAnonymous       bool                  `mapstructure:"allowAnonymous" yaml:"allowAnonymous"`
	WriteMode            string                `mapstructure:"writeMode" yaml:"writeMode"`
	ConfirmationClientID string                `mapstructure:"confirmationClientId" yaml:"confirmationClientId"`
	PublicURL            string                `mapstructure:"publicUrl" yaml:"publicUrl"`
	AuthorizationServers []string              `mapstructure:"authorizationServers" yaml:"authorizationServers"`
	TemplateDirectory    string                `mapstructure:"templateDirectory" yaml:"templateDirectory"`
	AllowedTemplates     []string              `mapstructure:"allowedTemplates" yaml:"allowedTemplates"`
	Profiles             map[string]MCPProfile `mapstructure:"profiles" yaml:"profiles"`
	Limits               MCPLimits             `mapstructure:"limits" yaml:"limits"`
}

// MCPProfile only restricts the caller's existing permissions.
type MCPProfile struct {
	Toolsets  []string `mapstructure:"toolsets" yaml:"toolsets"`
	DenyTools []string `mapstructure:"denyTools" yaml:"denyTools"`
}

// MCPLimits bounds work and response sizes independently of client prompts.
type MCPLimits struct {
	MaxResultBytes     int   `mapstructure:"maxResultBytes" yaml:"maxResultBytes"`
	MaxRequestBytes    int64 `mapstructure:"maxRequestBytes" yaml:"maxRequestBytes"`
	MaxItemsPerPage    int   `mapstructure:"maxItemsPerPage" yaml:"maxItemsPerPage"`
	MaxScanItems       int   `mapstructure:"maxScanItems" yaml:"maxScanItems"`
	MaxInternalCalls   int   `mapstructure:"maxInternalCalls" yaml:"maxInternalCalls"`
	CallsPerMinute     int   `mapstructure:"callsPerMinute" yaml:"callsPerMinute"`
	MaxConcurrentCalls int   `mapstructure:"maxConcurrentCalls" yaml:"maxConcurrentCalls"`
	TimeoutSeconds     int   `mapstructure:"timeoutSeconds" yaml:"timeoutSeconds"`
	PlanTTLSeconds     int   `mapstructure:"planTTLSeconds" yaml:"planTTLSeconds"`
}

func setMCPDefaults(v *viper.Viper) {
	defaults := map[string]any{
		"enabled": false, "path": "/mcp", "allowAnonymous": false, "writeMode": "disabled",
		"publicUrl": "", "confirmationClientId": "", "authorizationServers": []string{}, "templateDirectory": "", "allowedTemplates": []string{},
		"profiles.default.toolsets":  []string{"discovery", "read", "docs"},
		"profiles.analyst.toolsets":  []string{"discovery", "read", "docs", "query"},
		"profiles.operator.toolsets": []string{"discovery", "read", "docs", "create"},
		"limits.maxResultBytes":      65536, "limits.maxRequestBytes": 1048576,
		"limits.maxItemsPerPage": 50, "limits.maxScanItems": 1000, "limits.maxInternalCalls": 100,
		"limits.callsPerMinute": 120, "limits.maxConcurrentCalls": 8, "limits.timeoutSeconds": 30, "limits.planTTLSeconds": 300,
	}
	for key, value := range defaults {
		v.SetDefault("mcp."+key, value)
	}
}

func applyMCPConfig(cfg *Config) error {
	if raw, ok := os.LookupEnv("BASYX_MCP_ENABLED"); ok {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("MCP-CONFIG-ENABLED: %w", err)
		}
		cfg.MCP.Enabled = enabled
	}
	return cfg.MCP.Validate()
}

// Validate rejects invalid operation modes and unbounded resource budgets.
func (c MCPConfig) Validate() error {
	if c.Path == "/" || !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Path, "?#%{}") || strings.HasSuffix(c.Path, "/") || strings.Contains(c.Path, "..") {
		return fmt.Errorf("MCP-CONFIG-PATH: use a non-root absolute path without a trailing slash")
	}
	if c.WriteMode != "disabled" && c.WriteMode != "confirm" {
		return fmt.Errorf("MCP-CONFIG-WRITEMODE: expected disabled or confirm")
	}
	if err := c.Limits.validate(); err != nil {
		return err
	}
	if err := validateMCPPublicURL(c.PublicURL); err != nil {
		return err
	}
	if c.Enabled && c.WriteMode == "confirm" && c.PublicURL == "" {
		return fmt.Errorf("MCP-CONFIG-CONFIRMATION: publicUrl is required for human approval")
	}
	return nil
}

func isLoopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }

func (l MCPLimits) validate() error {
	if l.MaxResultBytes < 1024 || l.MaxRequestBytes < 1024 {
		return fmt.Errorf("MCP-CONFIG-LIMITS: byte budgets must be at least 1024")
	}
	for _, budget := range []int{l.MaxItemsPerPage, l.MaxScanItems, l.MaxInternalCalls, l.CallsPerMinute, l.MaxConcurrentCalls, l.TimeoutSeconds, l.PlanTTLSeconds} {
		if budget < 1 {
			return fmt.Errorf("MCP-CONFIG-LIMITS: all budgets must be positive")
		}
	}
	return nil
}
func validateMCPPublicURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("MCP-CONFIG-PUBLICURL: expected a URL without credentials, query or fragment")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("MCP-CONFIG-PUBLICURL: expected HTTP(S)")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("MCP-CONFIG-PUBLICURL: HTTP is allowed only on loopback; use HTTPS")
	}
	return nil
}
