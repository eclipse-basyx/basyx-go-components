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

func TestParseReferences_EmptyJSON(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)
	emptyJSON := json.RawMessage("[]")

	refs, err := builder.ParseReferences(emptyJSON, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 0 {
		t.Errorf("Expected 0 references, got %d", len(refs))
	}

	if len(referenceBuilderRefs) != 0 {
		t.Errorf("Expected 0 builders in map, got %d", len(referenceBuilderRefs))
	}
}

func TestParseReferences_SingleReference(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	jsonData := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/concept"
		}
	]`)

	refs, err := builder.ParseReferences(jsonData, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("Expected 1 reference, got %d", len(refs))
	}

	ref := refs[0]
	if string(ref.Type) != "ExternalReference" {
		t.Errorf("Expected type 'ExternalReference', got '%s'", ref.Type)
	}

	if len(ref.Keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(ref.Keys))
	}

	key := ref.Keys[0]
	if string(key.Type) != "GlobalReference" {
		t.Errorf("Expected key type 'GlobalReference', got '%s'", key.Type)
	}

	if key.Value != "https://example.com/concept" {
		t.Errorf("Expected key value 'https://example.com/concept', got '%s'", key.Value)
	}

	if len(referenceBuilderRefs) != 1 {
		t.Errorf("Expected 1 builder in map, got %d", len(referenceBuilderRefs))
	}

	if _, exists := referenceBuilderRefs[100]; !exists {
		t.Error("Expected builder with ID 100 in map")
	}
}

func TestParseReferences_MultipleKeysPerReference(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	jsonData := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ModelReference",
			"key_id": 1,
			"key_type": "Submodel",
			"key_value": "https://example.com/submodel/123"
		},
		{
			"reference_id": 100,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "Property",
			"key_value": "Temperature"
		}
	]`)

	refs, err := builder.ParseReferences(jsonData, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("Expected 1 reference, got %d", len(refs))
	}

	ref := refs[0]
	if len(ref.Keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(ref.Keys))
	}

	// Verify first key
	if ref.Keys[0].Value != "https://example.com/submodel/123" {
		t.Errorf("Expected first key value 'https://example.com/submodel/123', got '%s'", ref.Keys[0].Value)
	}

	// Verify second key
	if ref.Keys[1].Value != "Temperature" {
		t.Errorf("Expected second key value 'Temperature', got '%s'", ref.Keys[1].Value)
	}
}

func TestParseReferences_MultipleReferences(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	jsonData := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/concept1"
		},
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	refs, err := builder.ParseReferences(jsonData, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("Expected 2 references, got %d", len(refs))
	}

	if len(referenceBuilderRefs) != 2 {
		t.Errorf("Expected 2 builders in map, got %d", len(referenceBuilderRefs))
	}
}

func TestParseReferences_NullKeys(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	jsonData := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": null,
			"key_type": null,
			"key_value": null
		}
	]`)

	refs, err := builder.ParseReferences(jsonData, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("Expected 1 reference, got %d", len(refs))
	}

	// Reference should exist but have no keys
	ref := refs[0]
	if len(ref.Keys) != 0 {
		t.Errorf("Expected 0 keys for reference with null key data, got %d", len(ref.Keys))
	}
}

func TestParseReferredReferences_EmptyJSON(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)
	emptyJSON := json.RawMessage("[]")

	err := builder.ParseReferredReferences(emptyJSON, referenceBuilderRefs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestParseReferredReferences_SingleLevel(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	// First create root reference
	rootJSON := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/root"
		}
	]`)

	refs, err := builder.ParseReferences(rootJSON, referenceBuilderRefs)
	if err != nil {
		t.Fatalf("Error creating root reference: %v", err)
	}

	// Now add referred reference
	referredJSON := json.RawMessage(`[
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

	err = builder.ParseReferredReferences(referredJSON, referenceBuilderRefs)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Build nested structure
	for _, rb := range referenceBuilderRefs {
		rb.BuildNestedStructure()
	}

	// Verify hierarchy
	rootRef := refs[0]
	if rootRef.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set")
	}

	if string(rootRef.ReferredSemanticID.Type) != "ModelReference" {
		t.Errorf("Expected ReferredSemanticID type 'ModelReference', got '%s'", rootRef.ReferredSemanticID.Type)
	}

	if len(rootRef.ReferredSemanticID.Keys) != 1 {
		t.Fatalf("Expected 1 key in ReferredSemanticID, got %d", len(rootRef.ReferredSemanticID.Keys))
	}
}

func TestParseReferredReferences_MultiLevel(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	// Create root reference
	rootJSON := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/root"
		}
	]`)

	refs, _ := builder.ParseReferences(rootJSON, referenceBuilderRefs)

	// Add two levels of referred references
	referredJSON := json.RawMessage(`[
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"parentReference": 100,
			"rootReference": 100,
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "Level1"
		},
		{
			"reference_id": 102,
			"reference_type": "ExternalReference",
			"parentReference": 101,
			"rootReference": 100,
			"key_id": 3,
			"key_type": "GlobalReference",
			"key_value": "Level2"
		}
	]`)

	err := builder.ParseReferredReferences(referredJSON, referenceBuilderRefs)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Build nested structure
	for _, rb := range referenceBuilderRefs {
		rb.BuildNestedStructure()
	}

	// Verify two-level hierarchy
	rootRef := refs[0]
	if rootRef.ReferredSemanticID == nil {
		t.Fatal("Expected level 1 ReferredSemanticId")
	}

	if rootRef.ReferredSemanticID.ReferredSemanticID == nil {
		t.Fatal("Expected level 2 ReferredSemanticId")
	}

	level2 := rootRef.ReferredSemanticID.ReferredSemanticID
	if level2.Keys[0].Value != "Level2" {
		t.Errorf("Expected level 2 key value 'Level2', got '%s'", level2.Keys[0].Value)
	}
}

func TestParseReferredReferences_MissingParent(t *testing.T) {
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	// Try to add referred reference without creating root first
	referredJSON := json.RawMessage(`[
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"parentReference": 999,
			"rootReference": 999,
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	err := builder.ParseReferredReferences(referredJSON, referenceBuilderRefs)
	if err == nil {
		t.Error("Expected error when parent reference not found")
	}
}

func TestParseLangStringNameType_Valid(t *testing.T) {
	jsonData := json.RawMessage(`[
		{
			"id": 1,
			"language": "en",
			"text": "Example Name"
		},
		{
			"id": 2,
			"language": "de",
			"text": "Beispielname"
		}
	]`)

	names, err := builder.ParseLangStringNameType(jsonData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("Expected 2 names, got %d", len(names))
	}

	// Check first name
	if names[0].Language != "en" {
		t.Errorf("Expected language 'en', got '%s'", names[0].Language)
	}

	if names[0].Text != "Example Name" {
		t.Errorf("Expected text 'Example Name', got '%s'", names[0].Text)
	}

	// Check second name
	if names[1].Language != "de" {
		t.Errorf("Expected language 'de', got '%s'", names[1].Language)
	}

	if names[1].Text != "Beispielname" {
		t.Errorf("Expected text 'Beispielname', got '%s'", names[1].Text)
	}
}

func TestParseLangStringNameType_Empty(t *testing.T) {
	jsonData := json.RawMessage(`[]`)

	names, err := builder.ParseLangStringNameType(jsonData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(names) != 0 {
		t.Errorf("Expected 0 names, got %d", len(names))
	}
}

func TestParseLangStringNameType_WithoutID(t *testing.T) {
	// Items without 'id' field should be skipped
	jsonData := json.RawMessage(`[
		{
			"language": "en",
			"text": "No ID"
		},
		{
			"id": 1,
			"language": "de",
			"text": "Has ID"
		}
	]`)

	names, err := builder.ParseLangStringNameType(jsonData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(names) != 1 {
		t.Fatalf("Expected 1 name (one without ID skipped), got %d", len(names))
	}

	if names[0].Text != "Has ID" {
		t.Errorf("Expected text 'Has ID', got '%s'", names[0].Text)
	}
}

func TestParseLangStringTextType_Valid(t *testing.T) {
	jsonData := json.RawMessage(`[
		{
			"id": 1,
			"language": "en",
			"text": "Example Description"
		},
		{
			"id": 2,
			"language": "de",
			"text": "Beispielbeschreibung"
		}
	]`)

	texts, err := builder.ParseLangStringTextType(jsonData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(texts) != 2 {
		t.Fatalf("Expected 2 texts, got %d", len(texts))
	}

	// Check first text
	if texts[0].GetLanguage() != "en" {
		t.Errorf("Expected language 'en', got '%s'", texts[0].GetLanguage())
	}

	if texts[0].GetText() != "Example Description" {
		t.Errorf("Expected text 'Example Description', got '%s'", texts[0].GetText())
	}

	// Check second text
	if texts[1].GetLanguage() != "de" {
		t.Errorf("Expected language 'de', got '%s'", texts[1].GetLanguage())
	}

	if texts[1].GetText() != "Beispielbeschreibung" {
		t.Errorf("Expected text 'Beispielbeschreibung', got '%s'", texts[1].GetText())
	}
}

func TestParseLangStringTextType_Empty(t *testing.T) {
	jsonData := json.RawMessage(`[]`)

	texts, err := builder.ParseLangStringTextType(jsonData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(texts) != 0 {
		t.Errorf("Expected 0 texts, got %d", len(texts))
	}
}

func TestIntegration_CompleteReferenceHierarchy(t *testing.T) {
	// This test simulates the complete flow of parsing references with hierarchies
	referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)

	// Step 1: Parse root references (semantic IDs and supplemental semantic IDs)
	rootRefsJSON := json.RawMessage(`[
		{
			"reference_id": 100,
			"reference_type": "ExternalReference",
			"key_id": 1,
			"key_type": "GlobalReference",
			"key_value": "https://example.com/concept"
		},
		{
			"reference_id": 200,
			"reference_type": "ModelReference",
			"key_id": 2,
			"key_type": "ConceptDescription",
			"key_value": "0173-1#01-ABC123#001"
		}
	]`)

	rootRefs, err := builder.ParseReferences(rootRefsJSON, referenceBuilderRefs)
	if err != nil {
		t.Fatalf("Error parsing root references: %v", err)
	}

	if len(rootRefs) != 2 {
		t.Fatalf("Expected 2 root references, got %d", len(rootRefs))
	}

	// Step 2: Parse referred references
	referredRefsJSON := json.RawMessage(`[
		{
			"reference_id": 101,
			"reference_type": "ModelReference",
			"parentReference": 100,
			"rootReference": 100,
			"key_id": 10,
			"key_type": "ConceptDescription",
			"key_value": "Child1"
		},
		{
			"reference_id": 102,
			"reference_type": "ExternalReference",
			"parentReference": 101,
			"rootReference": 100,
			"key_id": 11,
			"key_type": "GlobalReference",
			"key_value": "Grandchild1"
		}
	]`)

	err = builder.ParseReferredReferences(referredRefsJSON, referenceBuilderRefs)
	if err != nil {
		t.Fatalf("Error parsing referred references: %v", err)
	}

	// Step 3: Build nested structures
	for _, rb := range referenceBuilderRefs {
		rb.BuildNestedStructure()
	}

	// Step 4: Verify the complete structure
	// First root reference should have a two-level hierarchy
	ref1 := rootRefs[0]
	if ref1.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID at level 1")
	}

	if ref1.ReferredSemanticID.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID at level 2")
	}

	// Second root reference should have no referred semantic IDs
	ref2 := rootRefs[1]
	if ref2.ReferredSemanticID != nil {
		t.Error("Expected no ReferredSemanticID for second root reference")
	}

	// Verify the complete hierarchy of first reference
	if ref1.Keys[0].Value != "https://example.com/concept" {
		t.Errorf("Unexpected root key value: %s", ref1.Keys[0].Value)
	}

	if ref1.ReferredSemanticID.Keys[0].Value != "Child1" {
		t.Errorf("Unexpected child key value: %s", ref1.ReferredSemanticID.Keys[0].Value)
	}

	if ref1.ReferredSemanticID.ReferredSemanticID.Keys[0].Value != "Grandchild1" {
		t.Errorf("Unexpected grandchild key value: %s", ref1.ReferredSemanticID.ReferredSemanticID.Keys[0].Value)
	}
}
