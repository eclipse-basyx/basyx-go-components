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
	"strings"
	"testing"
)

func TestReBACConfigDefaultsToDisabled(t *testing.T) {
	unsetReBACEnv(t)
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ReBAC.Enabled {
		t.Fatal("expected ReBAC to be disabled by default")
	}
	if cfg.ReBAC.TimeoutSeconds != defaultReBACTimeoutSeconds {
		t.Fatalf("timeout default = %d, want %d", cfg.ReBAC.TimeoutSeconds, defaultReBACTimeoutSeconds)
	}
}

func TestLoadConfigAppliesReBACEnvironment(t *testing.T) {
	unsetReBACEnv(t)
	t.Setenv("REBAC_ENABLED", "true")
	t.Setenv("REBAC_URL", "http://openfga:8080/")
	t.Setenv("REBAC_STORE_ID", "store-1")
	t.Setenv("REBAC_MODEL_ID", "model-1")
	t.Setenv("REBAC_SCOPE", "demo")
	t.Setenv("REBAC_TOKEN", "service-token")
	t.Setenv("REBAC_TIMEOUT_SECONDS", "12")
	t.Setenv("REBAC_ADMINISTRATORS", `[{"type":"user","issuer":"https://issuer.example","subject":"alice"}]`)
	t.Setenv("REBAC_GROUP_CLAIM_MAPPINGS", `[{"issuer":"https://issuer.example","claim":"groups"}]`)

	cfg, err := LoadConfig(writeTempConfig(t, "abac:\n  enabled: true\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ReBAC.Enabled || cfg.ReBAC.URL != "http://openfga:8080" || cfg.ReBAC.TimeoutSeconds != 12 || cfg.ReBAC.Token != "service-token" {
		t.Fatalf("unexpected ReBAC config: %+v", cfg.ReBAC)
	}
	if len(cfg.ReBAC.Administrators) != 1 || cfg.ReBAC.Administrators[0].Type != "user" {
		t.Fatalf("unexpected administrators: %+v", cfg.ReBAC.Administrators)
	}
	if len(cfg.ReBAC.GroupClaimMappings) != 1 || cfg.ReBAC.GroupClaimMappings[0].Claim != "groups" {
		t.Fatalf("unexpected group claim mappings: %+v", cfg.ReBAC.GroupClaimMappings)
	}
}

func TestLoadConfigRejectsWhitespaceReBACTokenEnvironment(t *testing.T) {
	unsetReBACEnv(t)
	t.Setenv("REBAC_TOKEN", " \t ")
	_, err := LoadConfig("")
	if err == nil || !strings.Contains(err.Error(), "CONFIG-REBAC-ENV-TOKEN") {
		t.Fatalf("LoadConfig() error = %v, want token environment error", err)
	}
}

func TestReBACConfigRejectsIncompleteOrUnsafeEnabledConfiguration(t *testing.T) {
	valid := ReBACConfig{
		Enabled:        true,
		URL:            "https://openfga.example",
		StoreID:        "store",
		ModelID:        "model",
		Scope:          "production",
		TimeoutSeconds: 5,
		Administrators: []ReBACAdministratorConfig{{Type: "user", Issuer: "https://issuer.example", Subject: "alice"}},
	}
	cases := []struct {
		name    string
		abac    bool
		mutate  func(*ReBACConfig)
		wantErr string
	}{
		{name: "abac disabled", wantErr: "CONFIG-REBAC-ABAC"},
		{name: "missing administrator", abac: true, mutate: func(c *ReBACConfig) { c.Administrators = nil }, wantErr: "CONFIG-REBAC-ADMINISTRATORS"},
		{name: "missing store", abac: true, mutate: func(c *ReBACConfig) { c.StoreID = " " }, wantErr: "CONFIG-REBAC-STOREID"},
		{name: "unsafe URL", abac: true, mutate: func(c *ReBACConfig) { c.URL = "https://user:secret@openfga.example" }, wantErr: "CONFIG-REBAC-URL"},
		{name: "timeout too high", abac: true, mutate: func(c *ReBACConfig) { c.TimeoutSeconds = maxReBACTimeoutSeconds + 1 }, wantErr: "CONFIG-REBAC-TIMEOUT"},
		{name: "invalid administrator", abac: true, mutate: func(c *ReBACConfig) {
			c.Administrators = []ReBACAdministratorConfig{{Type: "service", Issuer: "issuer", Subject: "subject"}}
		}, wantErr: "CONFIG-REBAC-ADMINISTRATORS"},
		{name: "duplicate group mapping", abac: true, mutate: func(c *ReBACConfig) {
			c.GroupClaimMappings = []ReBACGroupClaimMappingConfig{{Issuer: "issuer", Claim: "groups"}, {Issuer: "issuer", Claim: "roles"}}
		}, wantErr: "CONFIG-REBAC-GROUPCLAIMS"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rebac := valid
			if testCase.mutate != nil {
				testCase.mutate(&rebac)
			}
			err := validateReBACConfig(&Config{ABAC: ABACConfig{Enabled: testCase.abac}, ReBAC: rebac})
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("validateReBACConfig() error = %v, want %s", err, testCase.wantErr)
			}
		})
	}
}

func TestReBACConfigAcceptsIssuerScopedPrincipals(t *testing.T) {
	cfg := &Config{
		ABAC: ABACConfig{Enabled: true},
		ReBAC: ReBACConfig{
			Enabled:        true,
			URL:            "https://openfga.example/",
			StoreID:        "store",
			ModelID:        "model",
			Scope:          "production",
			TimeoutSeconds: 5,
			Administrators: []ReBACAdministratorConfig{
				{Type: "USER", Issuer: "https://issuer.example", Subject: "alice"},
				{Type: "group", Issuer: "https://issuer.example", Subject: "operators"},
			},
			GroupClaimMappings: []ReBACGroupClaimMappingConfig{{Issuer: "https://issuer.example", Claim: "groups"}},
		},
	}
	if err := validateReBACConfig(cfg); err != nil {
		t.Fatalf("validate ReBAC config: %v", err)
	}
	if cfg.ReBAC.URL != "https://openfga.example" || cfg.ReBAC.Administrators[0].Type != "user" {
		t.Fatalf("expected normalized config, got %+v", cfg.ReBAC)
	}
}

func unsetReBACEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"REBAC_ENABLED",
		"REBAC_URL",
		"REBAC_STORE_ID",
		"REBAC_MODEL_ID",
		"REBAC_SCOPE",
		"REBAC_TOKEN",
		"REBAC_TIMEOUT_SECONDS",
		"REBAC_ADMINISTRATORS",
		"REBAC_GROUP_CLAIM_MAPPINGS",
	} {
		withUnsetEnv(t, key)
	}
}
