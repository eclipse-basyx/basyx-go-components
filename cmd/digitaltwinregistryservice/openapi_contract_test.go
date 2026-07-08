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

package main

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIDocumentsShellDescriptorAssetIDs(t *testing.T) {
	spec, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	operation := shellDescriptorsGetOperation(t, string(spec))
	for _, want := range []string{
		"../Part2-API-Schemas/openapi.yaml#/components/parameters/AssetIds",
		"#/components/parameters/DTRCreatedAfter",
		"Supports assetIds query parameters",
	} {
		if !strings.Contains(operation, want) {
			t.Fatalf("GET /shell-descriptors is missing %q", want)
		}
	}
}

func shellDescriptorsGetOperation(t *testing.T, spec string) string {
	t.Helper()

	pathIndex := strings.Index(spec, "  /shell-descriptors:\n")
	if pathIndex < 0 {
		t.Fatal("missing /shell-descriptors path")
	}
	getIndex := strings.Index(spec[pathIndex:], "    get:\n")
	if getIndex < 0 {
		t.Fatal("missing GET /shell-descriptors operation")
	}
	operationStart := pathIndex + getIndex
	operationEnd := strings.Index(spec[operationStart:], "    post:\n")
	if operationEnd < 0 {
		t.Fatal("missing POST /shell-descriptors operation boundary")
	}

	return spec[operationStart : operationStart+operationEnd]
}
