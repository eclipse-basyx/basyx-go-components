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

import "fmt"

// AuthorizationResourceBoundFirst selects resource policies before object-based ABAC.
const AuthorizationResourceBoundFirst = "resource-bound-first"

// AuthorizationLegacyABAC preserves the existing object-based security setup.
const AuthorizationLegacyABAC = "legacy-abac"

// SecurityConfig selects the authorization strategy.
type SecurityConfig struct {
	AuthorizationMode string `mapstructure:"authorizationMode" yaml:"authorizationMode"`
}

// AccessPrincipal identifies a verified subject within its issuer namespace.
type AccessPrincipal struct {
	Issuer  string `json:"issuer" mapstructure:"issuer" yaml:"issuer"`
	Subject string `json:"subject" mapstructure:"subject" yaml:"subject"`
}

// ReBACConfig configures resource policies and initial ownership.
type ReBACConfig struct {
	PolicyScope    string          `mapstructure:"policyScope" yaml:"policyScope"`
	ModelPath      string          `mapstructure:"modelPath" yaml:"modelPath"`
	BootstrapOwner AccessPrincipal `mapstructure:"bootstrapOwner" yaml:"bootstrapOwner"`
}

// ResourceBoundEnabled reports whether resource-bound authorization is selected.
func ResourceBoundEnabled(cfg *Config) bool {
	return cfg != nil && cfg.Security.AuthorizationMode == AuthorizationResourceBoundFirst
}
func applyResourceBoundEnvOverrides(cfg *Config) {
	fields := []struct {
		target *string
		key    string
	}{
		{&cfg.Security.AuthorizationMode, "SECURITY_AUTHORIZATION_MODE"},
		{&cfg.ReBAC.PolicyScope, "REBAC_POLICY_SCOPE"}, {&cfg.ReBAC.ModelPath, "REBAC_MODEL_PATH"},
		{&cfg.ReBAC.BootstrapOwner.Issuer, "REBAC_BOOTSTRAP_OWNER_ISSUER"},
		{&cfg.ReBAC.BootstrapOwner.Subject, "REBAC_BOOTSTRAP_OWNER_SUBJECT"},
	}
	for _, field := range fields {
		if value, ok := lookupFirstTrimmedEnv(field.key, "BASYX_"+field.key); ok {
			*field.target = value
		}
	}
}
func validateResourceBoundConfig(cfg *Config) error {
	switch cfg.Security.AuthorizationMode {
	case "", AuthorizationLegacyABAC:
		return nil
	case AuthorizationResourceBoundFirst:
	default:
		return fmt.Errorf("CONFIG-REBAC-MODE unsupported authorization mode %q", cfg.Security.AuthorizationMode)
	}
	if cfg.ReBAC.PolicyScope == "" {
		cfg.ReBAC.PolicyScope = "default"
	}
	scope, err := normalizeABACPolicyScope(cfg.ReBAC.PolicyScope)
	if err != nil {
		return fmt.Errorf("CONFIG-REBAC-SCOPE %w", err)
	}
	cfg.ReBAC.PolicyScope = scope
	if cfg.ReBAC.BootstrapOwner.Issuer == "" || cfg.ReBAC.BootstrapOwner.Subject == "" {
		return fmt.Errorf("CONFIG-REBAC-OWNER bootstrap issuer and subject are required")
	}
	return nil
}
