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
	"strings"
	"testing"
)

func TestLoadConfigKeepsReBACDisabledByDefault(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	rebac := cfg.ReBAC
	if rebac.Enabled || rebac.ProvisionModel || rebac.Required() {
		t.Fatalf("ReBAC must be disabled by default: %#v", rebac)
	}
	if rebac.OpenFGA.TimeoutMillis != 2000 || rebac.OpenFGA.BatchCheckMaxItems != 50 ||
		rebac.ListObjectsMaxResults != 1000 || rebac.MaxScanCandidates != 5000 ||
		rebac.GroupClaim != "groups" || rebac.OpenFGA.Consistency != ReBACConsistencyHigher ||
		rebac.OpenFGA.Credentials.Method != ReBACCredentialsNone || rebac.Scope != "default" {
		t.Fatalf("unexpected ReBAC defaults: %#v", rebac)
	}
}

func TestLoadConfigAppliesReBACEnvOverrides(t *testing.T) {
	t.Setenv("REBAC_ENABLED", "true")
	t.Setenv("REBAC_SCOPE", "plant-a")
	t.Setenv("REBAC_OPENFGA_API_URL", "http://openfga:8080/")
	t.Setenv("REBAC_OPENFGA_STORE_ID", "01STORE")
	t.Setenv("REBAC_OPENFGA_AUTHORIZATION_MODEL_ID", "01MODEL")
	t.Setenv("REBAC_OPENFGA_CREDENTIALS_METHOD", "apiToken")
	t.Setenv("REBAC_OPENFGA_CREDENTIALS_API_TOKEN", "secret")
	t.Setenv("REBAC_OPENFGA_TIMEOUT_MILLIS", "750")
	t.Setenv("REBAC_OPENFGA_CONSISTENCY", "minimize_latency")
	t.Setenv("REBAC_LIST_OBJECTS_MAX_RESULTS", "10")
	t.Setenv("REBAC_MAX_SCAN_CANDIDATES", "20")
	t.Setenv("REBAC_GROUP_CLAIM", "basyx.groups")
	t.Setenv("REBAC_ADMINISTRATORS", "https://idp|admin-sub, https://idp|group:ops")
	t.Setenv("REBAC_PROVISION_MODEL", "true")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	rebac := cfg.ReBAC
	if !rebac.Enabled || rebac.Scope != "plant-a" || rebac.OpenFGA.APIURL != "http://openfga:8080" ||
		rebac.OpenFGA.StoreID != "01STORE" || rebac.OpenFGA.AuthorizationModelID != "01MODEL" ||
		rebac.OpenFGA.Credentials.APIToken != "secret" || rebac.OpenFGA.TimeoutMillis != 750 ||
		rebac.OpenFGA.Consistency != ReBACConsistencyMinimizeLatency || rebac.ListObjectsMaxResults != 10 ||
		rebac.MaxScanCandidates != 20 || rebac.GroupClaim != "basyx.groups" || !rebac.ProvisionModel {
		t.Fatalf("ReBAC env overrides not applied: %#v", rebac)
	}
	if len(rebac.Administrators) != 2 || rebac.Administrators[1] != "https://idp|group:ops" {
		t.Fatalf("administrators not parsed: %#v", rebac.Administrators)
	}
}

func TestLoadConfigRejectsInvalidReBACSettings(t *testing.T) {
	for name, test := range map[string]struct {
		env  map[string]string
		code string
	}{
		"missing api url":      {env: map[string]string{}, code: "CONFIG-REBAC-APIURL"},
		"relative api url":     {env: map[string]string{"REBAC_OPENFGA_API_URL": "openfga:8080"}, code: "CONFIG-REBAC-APIURL"},
		"latest model":         {env: map[string]string{"REBAC_OPENFGA_AUTHORIZATION_MODEL_ID": "latest"}, code: "CONFIG-REBAC-MODELID"},
		"bad scope":            {env: map[string]string{"REBAC_SCOPE": "a b"}, code: "CONFIG-REBAC-SCOPE"},
		"bad consistency":      {env: map[string]string{"REBAC_OPENFGA_CONSISTENCY": "EVENTUAL"}, code: "CONFIG-REBAC-CONSISTENCY"},
		"batch check too big":  {env: map[string]string{"REBAC_OPENFGA_BATCH_CHECK_MAX_ITEMS": "51"}, code: "CONFIG-REBAC-BATCHCHECK"},
		"token without secret": {env: map[string]string{"REBAC_OPENFGA_CREDENTIALS_METHOD": "apiToken"}, code: "CONFIG-REBAC-CREDENTIALS"},
		"unknown credentials":  {env: map[string]string{"REBAC_OPENFGA_CREDENTIALS_METHOD": "basic"}, code: "CONFIG-REBAC-CREDENTIALS"},
		"bad administrator":    {env: map[string]string{"REBAC_ADMINISTRATORS": "no-separator"}, code: "CONFIG-REBAC-ADMINISTRATORS"},
		"empty admin group":    {env: map[string]string{"REBAC_ADMINISTRATORS": "https://idp|group:"}, code: "CONFIG-REBAC-ADMINISTRATORS"},
		"zero scan candidates": {env: map[string]string{"REBAC_MAX_SCAN_CANDIDATES": "0"}, code: "CONFIG-REBAC-MAXSCAN"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("REBAC_ENABLED", "true")
			if _, apiURLOverridden := test.env["REBAC_OPENFGA_API_URL"]; !apiURLOverridden && test.code != "CONFIG-REBAC-APIURL" {
				t.Setenv("REBAC_OPENFGA_API_URL", "http://openfga:8080")
			}
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			_, err := LoadConfig("")
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}

func TestLoadConfigSkipsReBACValidationWhenDisabled(t *testing.T) {
	t.Setenv("REBAC_OPENFGA_CONSISTENCY", "EVENTUAL")
	if _, err := LoadConfig(""); err != nil {
		t.Fatalf("disabled ReBAC must not validate OpenFGA settings: %v", err)
	}
}
