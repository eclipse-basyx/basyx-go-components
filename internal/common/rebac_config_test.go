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
	if cfg.ReBAC.Enabled || cfg.ReBAC.GroupClaim != "groups" || len(cfg.ReBAC.Administrators) != 0 {
		t.Fatalf("unexpected ReBAC defaults: %#v", cfg.ReBAC)
	}
}

func TestLoadConfigAppliesReBACEnvOverrides(t *testing.T) {
	t.Setenv("REBAC_ENABLED", "true")
	t.Setenv("REBAC_GROUP_CLAIM", "basyx.groups")
	t.Setenv("REBAC_ADMINISTRATORS", "https://idp|admin-sub, https://idp|group:ops")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.ReBAC.Enabled || cfg.ReBAC.GroupClaim != "basyx.groups" {
		t.Fatalf("ReBAC env overrides not applied: %#v", cfg.ReBAC)
	}
	if len(cfg.ReBAC.Administrators) != 2 || cfg.ReBAC.Administrators[1] != "https://idp|group:ops" {
		t.Fatalf("administrators not parsed: %#v", cfg.ReBAC.Administrators)
	}
}

func TestLoadConfigRejectsInvalidReBACSettings(t *testing.T) {
	for name, test := range map[string]struct {
		env  map[string]string
		code string
	}{
		"bad administrator": {env: map[string]string{"REBAC_ADMINISTRATORS": "no-separator"}, code: "CONFIG-REBAC-ADMINISTRATORS"},
		"empty admin group": {env: map[string]string{"REBAC_ADMINISTRATORS": "https://idp|group:"}, code: "CONFIG-REBAC-ADMINISTRATORS"},
		"empty group claim": {env: map[string]string{"REBAC_GROUP_CLAIM": " "}, code: "CONFIG-REBAC-GROUPCLAIM"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("REBAC_ENABLED", "true")
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
	t.Setenv("REBAC_ADMINISTRATORS", "no-separator")
	if _, err := LoadConfig(""); err != nil {
		t.Fatalf("disabled ReBAC must not validate its settings: %v", err)
	}
}
