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

package rebac

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthorizationModelContainsEverySupportedResourceType(t *testing.T) {
	raw, err := AuthorizationModelJSON()
	if err != nil {
		t.Fatalf("build authorization model: %v", err)
	}
	var model struct {
		SchemaVersion   string `json:"schema_version"`
		TypeDefinitions []struct {
			Type      string         `json:"type"`
			Relations map[string]any `json:"relations"`
		} `json:"type_definitions"`
	}
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("decode authorization model: %v", err)
	}
	if model.SchemaVersion != "1.1" {
		t.Fatalf("schema version = %q, want 1.1", model.SchemaVersion)
	}
	types := map[string]map[string]any{}
	for _, definition := range model.TypeDefinitions {
		types[definition.Type] = definition.Relations
	}
	for _, name := range []string{"user", "group", "repository", "aas", "submodel", "element", "aas_descriptor", "submodel_descriptor", "discovery", "concept_description", "package"} {
		if _, ok := types[name]; !ok {
			t.Fatalf("missing type definition %q", name)
		}
	}
	for _, name := range []string{"read", "update", "create_child", "execute", "delete", "manage"} {
		if _, ok := types["element"][name]; !ok {
			t.Fatalf("element is missing %q permission", name)
		}
	}
	if _, ok := types["repository"]["create"]; !ok {
		t.Fatal("repository is missing create permission")
	}
	assertModelInheritance(t, types)
}

func assertModelInheritance(t *testing.T, types map[string]map[string]any) {
	t.Helper()
	for _, permission := range []string{"read", "update", "create_child", "execute", "delete", "manage"} {
		if !strings.Contains(relationJSON(t, types["element"][permission]), `"parent"`) {
			t.Fatalf("element.%s does not inherit from its parent", permission)
		}
	}
	if !strings.Contains(relationJSON(t, types["submodel"]["read"]), `"aas_parent"`) {
		t.Fatal("submodel.read does not inherit from aas_parent")
	}
	if strings.Contains(relationJSON(t, types["submodel"]["delete"]), `"aas_parent"`) {
		t.Fatal("submodel.delete must not inherit from aas_parent")
	}
	if !strings.Contains(relationJSON(t, types["aas_descriptor"]["read"]), `"source"`) || strings.Contains(relationJSON(t, types["aas_descriptor"]["update"]), `"source"`) {
		t.Fatal("descriptor source inheritance must be read-only")
	}
}

func TestValidateAuthorizationModelIgnoresServerID(t *testing.T) {
	raw, err := AuthorizationModelJSON()
	if err != nil {
		t.Fatalf("build authorization model: %v", err)
	}
	var model map[string]any
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	model["id"] = "01HMODEL"
	withID, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("encode model: %v", err)
	}
	if err := ValidateAuthorizationModel(withID); err != nil {
		t.Fatalf("validate model with server id: %v", err)
	}
}

func TestValidateAuthorizationModelRejectsStructuralDrift(t *testing.T) {
	raw, err := AuthorizationModelJSON()
	if err != nil {
		t.Fatalf("build authorization model: %v", err)
	}
	var model map[string]any
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	model["schema_version"] = "1.0"
	changed, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("encode model: %v", err)
	}
	err = ValidateAuthorizationModel(changed)
	if err == nil || !strings.Contains(err.Error(), "REBAC-MODEL-MISMATCH") {
		t.Fatalf("ValidateAuthorizationModel() error = %v, want mismatch", err)
	}
}

func relationJSON(t *testing.T, relation any) string {
	t.Helper()
	raw, err := json.Marshal(relation)
	if err != nil {
		t.Fatalf("encode relation: %v", err)
	}
	return string(raw)
}
