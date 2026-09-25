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

package model

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/openfga/language/pkg/go/transformer"
)

func TestCompiledModelMatchesReviewedDSL(t *testing.T) {
	t.Parallel()

	transformed, err := transformer.TransformDSLToJSON(DSL())
	if err != nil {
		t.Fatalf("model.fga does not compile: %v", err)
	}
	fromDSL, err := CanonicalHash([]byte(transformed))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := Hash()
	if err != nil {
		t.Fatal(err)
	}
	if fromDSL != embedded {
		t.Fatal("model.json is stale; regenerate it from model.fga")
	}
}

func TestModelStoredByOpenFGAHashesEqualToTheEmbeddedModel(t *testing.T) {
	t.Parallel()

	stored, err := os.ReadFile("testdata/openfga_1_18_stored_model.json")
	if err != nil {
		t.Fatal(err)
	}
	storedHash, err := CanonicalHash(stored)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := Hash()
	if err != nil {
		t.Fatal(err)
	}
	if storedHash != embedded {
		t.Fatal("the model as returned by OpenFGA 1.18 must hash equal to the embedded model")
	}
}

func TestCanonicalHashIgnoresStoreAssignedFields(t *testing.T) {
	t.Parallel()

	var parsed map[string]any
	if err := json.Unmarshal(JSON(), &parsed); err != nil {
		t.Fatal(err)
	}
	parsed["id"] = "01HSTOREASSIGNED"
	parsed["conditions"] = map[string]any{}
	stored, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	storedHash, err := CanonicalHash(stored)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := Hash()
	if err != nil {
		t.Fatal(err)
	}
	if storedHash != embedded {
		t.Fatal("store-assigned fields must not change the model hash")
	}

	parsed["schema_version"] = "1.2"
	changed, _ := json.Marshal(parsed)
	changedHash, err := CanonicalHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == embedded {
		t.Fatal("semantic changes must change the model hash")
	}
}
