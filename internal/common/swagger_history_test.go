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
	"io/fs"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

const part2DateParameterRef = "../Part2-API-Schemas/openapi.yaml#/components/parameters/Date"

type historyOperation struct {
	path        string
	operationID string
}

var (
	aasHistoryOperation      = historyOperation{path: "/shells/{aasIdentifier}/$history", operationID: "GetAssetAdministrationShellVersionByIdAndDate"}
	submodelHistoryOperation = historyOperation{path: "/submodels/{submodelIdentifier}/$history", operationID: "GetSubmodelVersionByIdAndDate"}
)

func TestPart2HistoryDateParameterIsOptional(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(part2SchemasFS, "swagger_part2_schemas/V3.2.0/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Components struct {
			Parameters map[string]struct {
				Name     string `yaml:"name"`
				Required bool   `yaml:"required"`
			} `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err = yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}
	date, ok := parsed.Components.Parameters["Date"]
	if !ok || date.Name != "date" {
		t.Fatal("expected Part 2 schema to define the date query parameter")
	}
	if date.Required {
		t.Fatal("expected the date query parameter to be optional")
	}
}

func TestHistoryOperationsMatchPart2(t *testing.T) {
	t.Parallel()

	for spec, operations := range map[string][]historyOperation{
		"../../cmd/aasrepositoryservice/openapi.yaml":      {aasHistoryOperation},
		"../../cmd/submodelrepositoryservice/openapi.yaml": {submodelHistoryOperation},
		"../../cmd/aasenvironmentservice/openapi.yaml":     {aasHistoryOperation, submodelHistoryOperation},
	} {
		paths := readOpenAPIPaths(t, spec)
		for _, expected := range operations {
			assertHistoryOperation(t, spec, paths, expected)
		}
	}
}

type openAPIOperation struct {
	OperationID string   `yaml:"operationId"`
	SemanticIDs []string `yaml:"x-semanticIds"`
	Parameters  []struct {
		Ref string `yaml:"$ref"`
	} `yaml:"parameters"`
}

type openAPIPathItem struct {
	Get *openAPIOperation `yaml:"get"`
}

func readOpenAPIPaths(t *testing.T, spec string) map[string]openAPIPathItem {
	t.Helper()
	content, err := os.ReadFile(spec) // #nosec G304 -- reads repository-owned OpenAPI specifications only
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Paths map[string]openAPIPathItem `yaml:"paths"`
	}
	if err = yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("%s: invalid YAML: %v", spec, err)
	}
	return parsed.Paths
}

func assertHistoryOperation(t *testing.T, spec string, paths map[string]openAPIPathItem, expected historyOperation) {
	t.Helper()
	operation := paths[expected.path].Get
	if operation == nil {
		t.Fatalf("%s: missing GET %s", spec, expected.path)
	}
	if operation.OperationID != expected.operationID {
		t.Fatalf("%s: expected operationId %s, got %s", spec, expected.operationID, operation.OperationID)
	}
	expectedSemanticID := "https://admin-shell.io/aas/API/" + expected.operationID + "/3/2"
	if len(operation.SemanticIDs) != 1 || operation.SemanticIDs[0] != expectedSemanticID {
		t.Fatalf("%s: expected semanticId %s, got %v", spec, expectedSemanticID, operation.SemanticIDs)
	}
	for _, parameter := range operation.Parameters {
		if parameter.Ref == part2DateParameterRef {
			return
		}
	}
	t.Fatalf("%s: GET %s must reference the Part 2 date parameter", spec, expected.path)
}
