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
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReBACManagementAPIIsDocumentedPerResourceFamily(t *testing.T) {
	t.Parallel()

	for spec, expected := range map[string]struct {
		present []string
		absent  []string
	}{
		"../../cmd/submodelrepositoryservice/openapi.yaml": {
			present: []string{"/submodels/{submodelIdentifier}/$access/grants", "/submodels/{submodelIdentifier}/$access/inheritance",
				"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$access/invitations"},
			absent: []string{"/shells/{aasIdentifier}/$access", "/concept-descriptions/{cdIdentifier}/$access"},
		},
		"../../cmd/aasrepositoryservice/openapi.yaml": {
			present: []string{"/shells/{aasIdentifier}/$access/grants", "/security/rebac/invitations/accept"},
			absent:  []string{"/submodels/{submodelIdentifier}/$access/grants"},
		},
		"../../cmd/aasenvironmentservice/openapi.yaml": {
			present: []string{"/shell-descriptors/{aasIdentifier}/$access/grants", "/submodel-descriptors/{submodelIdentifier}/$access",
				"/lookup/shells/{aasIdentifier}/$access/effective", "/shells/{aasIdentifier}/$access"},
		},
		"../../cmd/discoveryservice/openapi.yaml": {
			present: []string{"/lookup/shells/{aasIdentifier}/$access/grants"},
			absent:  []string{"/shells/{aasIdentifier}/$access", "/shell-descriptors/{aasIdentifier}/$access"},
		},
	} {
		injected, parsed := injectedReBACSpec(t, spec)
		paths := parsed["paths"].(map[string]any)
		for _, path := range expected.present {
			if _, ok := paths[path]; !ok {
				t.Fatalf("%s: missing %s", spec, path)
			}
		}
		for _, path := range expected.absent {
			if _, ok := paths[path]; ok {
				t.Fatalf("%s: unexpected %s", spec, path)
			}
		}
		if string(injectReBACManagementAPI(injected)) != string(injected) {
			t.Fatalf("%s: injection must be idempotent", spec)
		}
		assertReBACComponents(t, spec, injected, parsed)
	}
}

func injectedReBACSpec(t *testing.T, spec string) ([]byte, map[string]any) {
	t.Helper()
	content, err := os.ReadFile(spec) // #nosec G304 -- reads repository-owned OpenAPI specifications only
	if err != nil {
		t.Fatal(err)
	}
	injected := injectReBACManagementAPI(content)
	var parsed map[string]any
	if err = yaml.Unmarshal(injected, &parsed); err != nil {
		t.Fatalf("%s: injected spec is not valid YAML: %v", spec, err)
	}
	return injected, parsed
}

func assertReBACComponents(t *testing.T, spec string, injected []byte, parsed map[string]any) {
	t.Helper()
	components := parsed["components"].(map[string]any)
	if _, ok := components["parameters"].(map[string]any)["ReBACIfMatch"]; !ok {
		t.Fatalf("%s: missing ReBAC parameters", spec)
	}
	if !strings.Contains(string(injected), "ReBACAccess:") {
		t.Fatalf("%s: missing ReBAC schemas", spec)
	}
}
