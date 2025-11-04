/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package builder_test

import (
	"encoding/json"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
)

func TestNewEmbeddedDataSpecificationsBuilder(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	if edsb == nil {
		t.Fatal("Expected non-nil builder")
	}
}

func TestEmbeddedDataSpecificationsBuilder_Build_Empty(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	result := edsb.Build()

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 specifications, got %d", len(result))
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_SingleEDS(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// JSON data for a single EDS with one reference
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/spec"
		}
	]`)

	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 1 {
		t.Fatalf("Expected 1 specification, got %d", len(result))
	}

	eds := result[0]
	if eds.DataSpecification == nil {
		t.Fatal("Expected DataSpecification to be set")
	}

	if string(eds.DataSpecification.Type) != "ExternalReference" {
		t.Errorf("Expected reference type 'ExternalReference', got '%s'", eds.DataSpecification.Type)
	}

	if len(eds.DataSpecification.Keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(eds.DataSpecification.Keys))
	}

	if eds.DataSpecification.Keys[0].Value != "https://example.com/spec" {
		t.Errorf("Expected key value 'https://example.com/spec', got '%s'", eds.DataSpecification.Keys[0].Value)
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_MultipleEDS(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// JSON data for multiple EDS
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/spec1"
		},
		{
			"eds_id": 2,
			"reference_id": 200,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 2 {
		t.Fatalf("Expected 2 specifications, got %d", len(result))
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_MultipleKeysPerEDS(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// JSON data for one EDS with multiple keys
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ModelReference",
			"key_id": 1,
			"key_type": "Submodel",
			"key_value": "https://example.com/submodel"
		},
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "Property",
			"key_value": "Temperature"
		}
	]`)

	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 1 {
		t.Fatalf("Expected 1 specification, got %d", len(result))
	}

	eds := result[0]
	if eds.DataSpecification == nil {
		t.Fatal("Expected DataSpecification to be set")
	}

	if len(eds.DataSpecification.Keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(eds.DataSpecification.Keys))
	}

	if eds.DataSpecification.Keys[0].Value != "https://example.com/submodel" {
		t.Errorf("Expected first key value 'https://example.com/submodel', got '%s'", eds.DataSpecification.Keys[0].Value)
	}

	if eds.DataSpecification.Keys[1].Value != "Temperature" {
		t.Errorf("Expected second key value 'Temperature', got '%s'", eds.DataSpecification.Keys[1].Value)
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_WithReferredReferences(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// JSON data for EDS with root reference
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/spec"
		}
	]`)

	// JSON data for referred reference
	edsReferredReferenceRows := json.RawMessage(`[
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"parentReference": 100,
			"rootReference": 100,
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 1 {
		t.Fatalf("Expected 1 specification, got %d", len(result))
	}

	eds := result[0]
	if eds.DataSpecification == nil {
		t.Fatal("Expected DataSpecification to be set")
	}

	if eds.DataSpecification.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set")
	}

	if string(eds.DataSpecification.ReferredSemanticID.Type) != "ModelReference" {
		t.Errorf("Expected ReferredSemanticID type 'ModelReference', got '%s'", eds.DataSpecification.ReferredSemanticID.Type)
	}

	if len(eds.DataSpecification.ReferredSemanticID.Keys) != 1 {
		t.Fatalf("Expected 1 key in ReferredSemanticID, got %d", len(eds.DataSpecification.ReferredSemanticID.Keys))
	}

	if eds.DataSpecification.ReferredSemanticID.Keys[0].Value != "0173-1#01-ABC123#001" {
		t.Errorf("Expected ReferredSemanticID key value '0173-1#01-ABC123#001', got '%s'", eds.DataSpecification.ReferredSemanticID.Keys[0].Value)
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_EmptyJSON(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	edsReferenceRows := json.RawMessage(`[]`)
	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 0 {
		t.Errorf("Expected 0 specifications, got %d", len(result))
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_InvalidJSON(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// Invalid JSON
	edsReferenceRows := json.RawMessage(`{invalid json}`)
	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestEmbeddedDataSpecificationsBuilder_BuildReferences_MultipleReferencesPerEDS_ShouldError(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// JSON data for one EDS with two different references (should be invalid)
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/spec1"
		},
		{
			"eds_id": 1,
			"reference_id": 200,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	edsReferredReferenceRows := json.RawMessage(`[]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err == nil {
		t.Error("Expected error when EDS has multiple different references")
	}
}

func TestEmbeddedDataSpecificationsBuilder_Integration_CompleteHierarchy(t *testing.T) {
	edsb := builder.NewEmbeddedDataSpecificationsBuilder()

	// Complex scenario: multiple EDS with references and hierarchies
	edsReferenceRows := json.RawMessage(`[
		{
			"eds_id": 1,
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/spec1"
		},
		{
			"eds_id": 2,
			"reference_id": 200,
			"reference_type": "ModelReference",
			"key_id": 10,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		},
		{
			"eds_id": 2,
			"reference_id": 200,
			"reference_type": "ModelReference",
			"key_id": 11,
			"key_type": "GlobalReference",
			"key_value": "https://eclass.com/concept"
		}
	]`)

	edsReferredReferenceRows := json.RawMessage(`[
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"parentReference": 100,
			"rootReference": 100,
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "Child1"
		},
		{
			"reference_id": 102,
			"reference_type": "ExternalReference",
			"parentReference": 101,
			"rootReference": 100,
			"key_id": 3,
			"key_type": "GlobalReference",
			"key_value": "Grandchild1"
		},
		{
			"reference_id": 201,
			"reference_type": "ExternalReference",
			"parentReference": 200,
			"rootReference": 200,
			"key_id": 20,
			"key_type": "GlobalReference",
			"key_value": "Child2"
		}
	]`)

	err := edsb.BuildReferences(edsReferenceRows, edsReferredReferenceRows)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := edsb.Build()

	if len(result) != 2 {
		t.Fatalf("Expected 2 specifications, got %d", len(result))
	}

	// Verify that at least one EDS has a multi-level hierarchy
	foundMultiLevel := false
	for _, eds := range result {
		if eds.DataSpecification != nil &&
			eds.DataSpecification.ReferredSemanticID != nil &&
			eds.DataSpecification.ReferredSemanticID.ReferredSemanticID != nil {
			foundMultiLevel = true

			// Verify the three-level hierarchy
			if eds.DataSpecification.Keys[0].Value != "https://example.com/spec1" {
				t.Errorf("Unexpected root key: %s", eds.DataSpecification.Keys[0].Value)
			}

			if eds.DataSpecification.ReferredSemanticID.Keys[0].Value != "Child1" {
				t.Errorf("Unexpected child key: %s", eds.DataSpecification.ReferredSemanticID.Keys[0].Value)
			}

			if eds.DataSpecification.ReferredSemanticID.ReferredSemanticID.Keys[0].Value != "Grandchild1" {
				t.Errorf("Unexpected grandchild key: %s", eds.DataSpecification.ReferredSemanticID.ReferredSemanticID.Keys[0].Value)
			}
		}
	}

	if !foundMultiLevel {
		t.Error("Expected to find at least one EDS with multi-level hierarchy")
	}
}
