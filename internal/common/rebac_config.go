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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package common

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const defaultReBACGroupClaim = "groups"

// ReBACConfig configures the experimental relationship-based access control.
//
// ReBAC extends ABAC as a strict union: a request is allowed when ABAC or
// ReBAC allows it. Relationships are stored and evaluated in the BaSyx
// PostgreSQL database; all services sharing a database share them.
type ReBACConfig struct {
	Enabled        bool     `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	GroupClaim     string   `mapstructure:"groupClaim" yaml:"groupClaim" json:"groupClaim"`
	Administrators []string `mapstructure:"administrators" yaml:"administrators" json:"administrators"`
}

func setReBACDefaults(v *viper.Viper) {
	v.SetDefault("rebac.enabled", false)
	v.SetDefault("rebac.groupClaim", defaultReBACGroupClaim)
	v.SetDefault("rebac.administrators", []string{})
}

func applyReBACEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value, ok := lookupFirstTrimmedEnv("REBAC_GROUP_CLAIM"); ok {
		cfg.ReBAC.GroupClaim = value
	}
	if value, ok := lookupFirstTrimmedEnv("REBAC_ADMINISTRATORS"); ok {
		cfg.ReBAC.Administrators = parseCommaSeparated(value)
	}
}

// validateReBACConfig normalizes the rebac section. Cross-feature
// requirements such as ABAC and OIDC are checked when a service starts.
func validateReBACConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-REBAC-NIL configuration must not be nil")
	}
	rebac := &cfg.ReBAC
	if !rebac.Enabled {
		return nil
	}
	rebac.GroupClaim = strings.TrimSpace(rebac.GroupClaim)
	if rebac.GroupClaim == "" {
		return fmt.Errorf("CONFIG-REBAC-GROUPCLAIM rebac.groupClaim must not be empty")
	}
	administrators, err := normalizeReBACAdministrators(rebac.Administrators)
	if err != nil {
		return err
	}
	rebac.Administrators = administrators
	return nil
}

// ReBACAdministrator is one configured bootstrap and recovery principal.
type ReBACAdministrator struct {
	Issuer  string
	Subject string
	Group   string
}

// ParseReBACAdministrator parses "issuer|subject" or "issuer|group:<name>".
func ParseReBACAdministrator(entry string) (ReBACAdministrator, error) {
	issuer, principal, found := strings.Cut(strings.TrimSpace(entry), "|")
	issuer = strings.TrimSpace(issuer)
	principal = strings.TrimSpace(principal)
	if !found || issuer == "" || principal == "" {
		return ReBACAdministrator{}, fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS entry %q must be issuer|subject or issuer|group:<name>", entry)
	}
	if group, isGroup := strings.CutPrefix(principal, "group:"); isGroup {
		group = strings.TrimSpace(group)
		if group == "" {
			return ReBACAdministrator{}, fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS entry %q has an empty group name", entry)
		}
		return ReBACAdministrator{Issuer: issuer, Group: group}, nil
	}
	return ReBACAdministrator{Issuer: issuer, Subject: principal}, nil
}

func normalizeReBACAdministrators(entries []string) ([]string, error) {
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		admin, err := ParseReBACAdministrator(entry)
		if err != nil {
			return nil, err
		}
		if admin.Group != "" {
			normalized = append(normalized, admin.Issuer+"|group:"+admin.Group)
			continue
		}
		normalized = append(normalized, admin.Issuer+"|"+admin.Subject)
	}
	return normalized, nil
}
