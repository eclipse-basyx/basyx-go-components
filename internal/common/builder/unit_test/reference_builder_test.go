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

//nolint:all
package builder_test

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestNewReferenceBuilder(t *testing.T) {
	refType := "ExternalReference"
	dbID := int64(123)

	ref, rb := builder.NewReferenceBuilder(refType, dbID)

	if ref == nil {
		t.Fatal("Expected non-nil reference")
	}

	if rb == nil {
		t.Fatal("Expected non-nil reference builder")
	}

	if string(ref.Type) != refType {
		t.Errorf("Expected reference type %s, got %s", refType, ref.Type)
	}

	if ref.Keys == nil {
		t.Error("Expected Keys slice to be initialized")
	}

	if len(ref.Keys) != 0 {
		t.Errorf("Expected empty Keys slice, got length %d", len(ref.Keys))
	}
}

func TestCreateKey(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ModelReference", 100)

	// Add first key
	rb.CreateKey(1, "Submodel", "https://example.com/submodel/123")

	if len(ref.Keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(ref.Keys))
	}

	key := ref.Keys[0]
	if string(key.Type) != "Submodel" {
		t.Errorf("Expected key type 'Submodel', got '%s'", key.Type)
	}

	if key.Value != "https://example.com/submodel/123" {
		t.Errorf("Expected key value 'https://example.com/submodel/123', got '%s'", key.Value)
	}

	// Add second key
	rb.CreateKey(2, "Property", "Temperature")

	if len(ref.Keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(ref.Keys))
	}

	key2 := ref.Keys[1]
	if string(key2.Type) != "Property" {
		t.Errorf("Expected key type 'Property', got '%s'", key2.Type)
	}

	if key2.Value != "Temperature" {
		t.Errorf("Expected key value 'Temperature', got '%s'", key2.Value)
	}
}

func TestCreateKey_DuplicatePrevention(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ModelReference", 100)

	// Add same key twice with same ID
	rb.CreateKey(1, "Submodel", "https://example.com/submodel/123")
	rb.CreateKey(1, "Submodel", "https://example.com/submodel/123")

	if len(ref.Keys) != 1 {
		t.Errorf("Expected 1 key (duplicate should be skipped), got %d", len(ref.Keys))
	}
}

func TestSetReferredSemanticID(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Create a referred semantic ID
	referredRef := &gen.Reference{
		Type: gen.ReferenceTypes("ModelReference"),
		Keys: []gen.Key{
			{Type: gen.KeyTypes("ConceptDescription"), Value: "0173-1#01-ABC123#001"},
		},
	}

	rb.SetReferredSemanticID(referredRef)

	if ref.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set")
	}

	if string(ref.ReferredSemanticID.Type) != "ModelReference" {
		t.Errorf("Expected ReferredSemanticID type 'ModelReference', got '%s'", ref.ReferredSemanticID.Type)
	}

	if len(ref.ReferredSemanticID.Keys) != 1 {
		t.Fatalf("Expected 1 key in ReferredSemanticID, got %d", len(ref.ReferredSemanticID.Keys))
	}
}

func TestCreateReferredSemanticID(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Create a ReferredSemanticID directly under the root
	rb.CreateReferredSemanticID(101, 100, "ModelReference")

	if ref.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set")
	}

	if string(ref.ReferredSemanticID.Type) != "ModelReference" {
		t.Errorf("Expected ReferredSemanticID type 'ModelReference', got '%s'", ref.ReferredSemanticID.Type)
	}
}

func TestCreateReferredSemanticIdKey(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Create a ReferredSemanticId
	rb.CreateReferredSemanticID(101, 100, "ModelReference")

	// Add a key to the ReferredSemanticId
	err := rb.CreateReferredSemanticIDKey(101, 1, "ConceptDescription", "0173-1#01-ABC123#001")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if ref.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set")
	}

	if len(ref.ReferredSemanticID.Keys) != 1 {
		t.Fatalf("Expected 1 key in ReferredSemanticID, got %d", len(ref.ReferredSemanticID.Keys))
	}

	key := ref.ReferredSemanticID.Keys[0]
	if string(key.Type) != "ConceptDescription" {
		t.Errorf("Expected key type 'ConceptDescription', got '%s'", key.Type)
	}

	if key.Value != "0173-1#01-ABC123#001" {
		t.Errorf("Expected key value '0173-1#01-ABC123#001', got '%s'", key.Value)
	}
}

func TestCreateReferredSemanticIdKey_NotFound(t *testing.T) {
	_, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Try to add a key to a non-existent ReferredSemanticId
	err := rb.CreateReferredSemanticIDKey(999, 1, "ConceptDescription", "0173-1#01-ABC123#001")
	if err == nil {
		t.Error("Expected error when adding key to non-existent ReferredSemanticId")
	}
}

func TestBuildNestedStructure_TwoLevels(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Add root key
	rb.CreateKey(1, "GlobalReference", "https://example.com/root")

	// Create first level ReferredSemanticId
	rb.CreateReferredSemanticID(101, 100, "ModelReference")
	err := rb.CreateReferredSemanticIDKey(101, 2, "ConceptDescription", "0173-1#01-ABC123#001")
	if err != nil {
		t.Error("Error creating first level ReferredSemanticId key:", err)
	}

	// Create second level ReferredSemanticId
	rb.CreateReferredSemanticID(102, 101, "ExternalReference")
	err = rb.CreateReferredSemanticIDKey(102, 3, "GlobalReference", "https://example.com/grandparent")
	if err != nil {
		t.Error("Error creating second level ReferredSemanticId key:", err)
	}

	// Build the nested structure
	rb.BuildNestedStructure()

	// Verify structure
	if ref.ReferredSemanticID == nil {
		t.Fatal("Expected ReferredSemanticID to be set at root level")
	}

	if ref.ReferredSemanticID.ReferredSemanticID == nil {
		t.Fatal("Expected nested ReferredSemanticID at second level")
	}

	// Check second level
	secondLevel := ref.ReferredSemanticID.ReferredSemanticID
	if string(secondLevel.Type) != "ExternalReference" {
		t.Errorf("Expected second level type 'ExternalReference', got '%s'", secondLevel.Type)
	}

	if len(secondLevel.Keys) != 1 {
		t.Fatalf("Expected 1 key at second level, got %d", len(secondLevel.Keys))
	}

	if secondLevel.Keys[0].Value != "https://example.com/grandparent" {
		t.Errorf("Expected key value 'https://example.com/grandparent', got '%s'", secondLevel.Keys[0].Value)
	}
}

func TestBuildNestedStructure_ThreeLevels(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Create three levels of nesting
	rb.CreateKey(1, "GlobalReference", "https://example.com/root")
	rb.CreateReferredSemanticID(101, 100, "ModelReference")
	err := rb.CreateReferredSemanticIDKey(101, 2, "ConceptDescription", "Level1")
	if err != nil {
		t.Error("Error creating first level ReferredSemanticId key:", err)
	}
	rb.CreateReferredSemanticID(102, 101, "ExternalReference")
	if err != nil {
		t.Error("Error creating second level ReferredSemanticId:", err)
	}
	err = rb.CreateReferredSemanticIDKey(102, 3, "GlobalReference", "Level2")
	if err != nil {
		t.Error("Error creating second level ReferredSemanticId key:", err)
	}
	rb.CreateReferredSemanticID(103, 102, "ModelReference")
	err = rb.CreateReferredSemanticIDKey(103, 4, "ConceptDescription", "Level3")
	if err != nil {
		t.Error("Error creating third level ReferredSemanticId key:", err)
	}

	rb.BuildNestedStructure()

	// Navigate through three levels
	level1 := ref.ReferredSemanticID
	if level1 == nil {
		t.Fatal("Expected level 1 ReferredSemanticId")
	}

	level2 := level1.ReferredSemanticID
	if level2 == nil {
		t.Fatal("Expected level 2 ReferredSemanticId")
	}

	level3 := level2.ReferredSemanticID
	if level3 == nil {
		t.Fatal("Expected level 3 ReferredSemanticId")
	}

	if level3.Keys[0].Value != "Level3" {
		t.Errorf("Expected level 3 key value 'Level3', got '%s'", level3.Keys[0].Value)
	}

	if level3.ReferredSemanticID != nil {
		t.Error("Expected no ReferredSemanticID at level 3")
	}
}

func TestBuildNestedStructure_EmptyBuilder(t *testing.T) {
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Just add a root key, no referred semantic IDs
	rb.CreateKey(1, "GlobalReference", "https://example.com/root")

	// Should not panic
	rb.BuildNestedStructure()

	if ref.ReferredSemanticID != nil {
		t.Error("Expected no ReferredSemanticID for simple reference")
	}
}

func TestCompleteReferenceHierarchy(t *testing.T) {
	// Create a complete reference hierarchy similar to real-world usage
	ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)

	// Root reference with multiple keys
	rb.CreateKey(1, "Submodel", "https://example.com/submodel/123")
	rb.CreateKey(2, "SubmodelElementCollection", "Measurements")
	rb.CreateKey(3, "Property", "Temperature")

	// First level ReferredSemanticId
	rb.CreateReferredSemanticID(201, 100, "ModelReference")
	err := rb.CreateReferredSemanticIDKey(201, 10, "ConceptDescription", "0173-1#01-ABC123#001")
	if err != nil {
		t.Error("Error creating first level ReferredSemanticId key:", err)
	}
	err = rb.CreateReferredSemanticIDKey(201, 11, "GlobalReference", "https://eclass.com/concept/123")
	if err != nil {
		t.Error("Error creating first level ReferredSemanticId key:", err)
	}

	// Second level ReferredSemanticId
	rb.CreateReferredSemanticID(202, 201, "ExternalReference")
	err = rb.CreateReferredSemanticIDKey(202, 20, "GlobalReference", "https://example.com/parent-concept")
	if err != nil {
		t.Error("Error creating second level ReferredSemanticId key:", err)
	}

	rb.BuildNestedStructure()

	// Verify root reference
	if len(ref.Keys) != 3 {
		t.Errorf("Expected 3 keys at root level, got %d", len(ref.Keys))
	}

	// Verify first level
	level1 := ref.ReferredSemanticID
	if level1 == nil {
		t.Fatal("Expected level 1 ReferredSemanticId")
	}

	if len(level1.Keys) != 2 {
		t.Errorf("Expected 2 keys at level 1, got %d", len(level1.Keys))
	}

	// Verify second level
	level2 := level1.ReferredSemanticID
	if level2 == nil {
		t.Fatal("Expected level 2 ReferredSemanticId")
	}

	if len(level2.Keys) != 1 {
		t.Errorf("Expected 1 key at level 2, got %d", len(level2.Keys))
	}

	if level2.Keys[0].Value != "https://example.com/parent-concept" {
		t.Errorf("Expected correct key value at level 2, got '%s'", level2.Keys[0].Value)
	}
}
