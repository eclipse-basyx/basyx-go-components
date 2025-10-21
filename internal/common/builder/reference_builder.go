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

// Package builder provides utilities for constructing complex AAS (Asset Administration Shell)
// data structures from database query results.
package builder

import (
	"fmt"
	"slices"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// ReferenceBuilder constructs Reference objects with nested ReferredSemanticId structures
// from flattened database rows. It handles the complexity of building hierarchical reference
// trees where references can contain other references as ReferredSemanticIds.
//
// The builder tracks database IDs to avoid duplicate entries and maintains relationships
// between parent and child references in the hierarchy.
type ReferenceBuilder struct {
	reference                  *gen.Reference               // The root reference being built
	keyIds                     []int64                      // Database IDs of keys already added to the root reference
	childKeyIds                []int64                      // Database IDs of keys in child references
	referredSemanticIdMap      map[int64]*ReferenceMetadata // Maps database IDs to reference metadata for hierarchy building
	referredSemanticIdBuilders map[int64]*ReferenceBuilder  // Maps database IDs to builders for nested references
	databaseId                 int64                        // Database ID of the root reference
}

// ReferenceMetadata holds metadata about a reference in the hierarchy, including
// its parent reference database ID and the reference object itself.
type ReferenceMetadata struct {
	parent    int64          // Database ID of the parent reference
	reference *gen.Reference // The reference object
}

// NewReferenceBuilder creates a new ReferenceBuilder instance and initializes a Reference
// object with the specified type and database ID.
//
// Parameters:
//   - referenceType: The type of reference (e.g., "ExternalReference", "ModelReference")
//   - dbId: The database ID of the reference for tracking and hierarchy building
//
// Returns:
//   - *gen.Reference: A pointer to the newly created Reference object
//   - *ReferenceBuilder: A pointer to the builder for constructing the reference
//
// Example:
//
//	ref, builder := NewReferenceBuilder("ExternalReference", 123)
//	builder.CreateKey(1, "GlobalReference", "https://example.com/concept")
func NewReferenceBuilder(referenceType string, dbId int64) (*gen.Reference, *ReferenceBuilder) {
	ref := &gen.Reference{
		Type: gen.ReferenceTypes(referenceType),
		Keys: []gen.Key{},
	}
	return ref, &ReferenceBuilder{keyIds: []int64{}, reference: ref, childKeyIds: []int64{}, databaseId: dbId, referredSemanticIdBuilders: make(map[int64]*ReferenceBuilder), referredSemanticIdMap: make(map[int64]*ReferenceMetadata)}
}

// CreateKey adds a new key to the root reference. Keys are the building blocks of a reference
// and define the path to the referenced element. Duplicate keys (based on database ID) are
// automatically skipped to prevent duplication when processing multiple database rows.
//
// Parameters:
//   - key_id: The database ID of the key for duplicate detection
//   - key_type: The type of key (e.g., "Submodel", "GlobalReference", "ConceptDescription")
//   - key_value: The value of the key (e.g., a URL or identifier)
//
// Example:
//
//	builder.CreateKey(1, "Submodel", "https://example.com/submodel/123")
//	builder.CreateKey(2, "SubmodelElementCollection", "MyCollection")
func (rb *ReferenceBuilder) CreateKey(key_id int64, key_type string, key_value string) {
	skip := slices.Contains(rb.keyIds, key_id)
	if !skip {
		rb.keyIds = append(rb.keyIds, key_id)
		rb.reference.Keys = append(rb.reference.Keys, gen.Key{
			Type:  gen.KeyTypes(key_type),
			Value: key_value,
		})
	}
}

// SetReferredSemanticId directly assigns a ReferredSemanticId to the root reference.
// This is used when the referred semantic ID is already constructed and needs to be
// attached to the reference.
//
// Parameters:
//   - referredSemanticId: A pointer to the Reference that should be set as the ReferredSemanticId
//
// Note: This method is typically used after the referred semantic ID has been fully
// constructed with all its keys and nested structure.
func (rb *ReferenceBuilder) SetReferredSemanticId(referredSemanticId *gen.Reference) {
	rb.reference.ReferredSemanticId = referredSemanticId
}

// CreateReferredSemanticId creates a new ReferredSemanticId reference within the hierarchy.
// ReferredSemanticIds can be nested, forming a tree structure where each reference can have
// its own ReferredSemanticId. This method handles creating new references and tracking their
// position in the hierarchy.
//
// Parameters:
//   - referredSemanticIdDbId: The database ID of the ReferredSemanticId reference
//   - parentId: The database ID of the parent reference in the hierarchy
//   - referenceType: The type of the ReferredSemanticId reference
//
// Returns:
//   - *ReferenceBuilder: Returns the builder instance for method chaining
//
// If the parentId matches the root reference's database ID, the ReferredSemanticId is
// immediately attached to the root reference. Otherwise, it's stored for later attachment
// during the BuildNestedStructure phase.
//
// Example:
//
//	// Create a ReferredSemanticId directly under the root reference
//	builder.CreateReferredSemanticId(456, 123, "ExternalReference")
//	// Create a nested ReferredSemanticId under another ReferredSemanticId
//	builder.CreateReferredSemanticId(789, 456, "ModelReference")
func (rb *ReferenceBuilder) CreateReferredSemanticId(referredSemanticIdDbId int64, parentId int64, referenceType string) *ReferenceBuilder {
	_, exists := rb.referredSemanticIdMap[referredSemanticIdDbId]
	if !exists {
		referredSemanticId, newBuilder := NewReferenceBuilder(referenceType, referredSemanticIdDbId)
		rb.referredSemanticIdBuilders[referredSemanticIdDbId] = newBuilder
		rb.referredSemanticIdMap[referredSemanticIdDbId] = &ReferenceMetadata{
			parent:    parentId,
			reference: referredSemanticId,
		}
		if parentId == rb.databaseId {
			rb.reference.ReferredSemanticId = referredSemanticId
		}
	}
	return rb
}

// CreateReferredSemanticIdKey adds a key to a specific ReferredSemanticId reference in the
// hierarchy. This method delegates to the appropriate builder for the target reference.
//
// Parameters:
//   - referredSemanticIdDbId: The database ID of the ReferredSemanticId to add the key to
//   - key_id: The database ID of the key for duplicate detection
//   - key_type: The type of key (e.g., "ConceptDescription", "GlobalReference")
//   - key_value: The value of the key
//
// Returns:
//   - error: Returns an error if the ReferredSemanticId builder cannot be found, nil otherwise
//
// This method must be called after CreateReferredSemanticId has been called for the
// corresponding referredSemanticIdDbId, otherwise it will return an error.
//
// Example:
//
//	builder.CreateReferredSemanticId(456, 123, "ExternalReference")
//	err := builder.CreateReferredSemanticIdKey(456, 1, "GlobalReference", "https://example.com")
//	if err != nil {
//	    // Handle error
//	}
func (rb *ReferenceBuilder) CreateReferredSemanticIdKey(referredSemanticIdDbId int64, key_id int64, key_type string, key_value string) error {
	builder, exists := rb.referredSemanticIdBuilders[referredSemanticIdDbId]
	if exists {
		builder.CreateKey(key_id, key_type, key_value)
	} else {
		fmt.Printf("[ReferenceBuilder:CreateReferredSemanticIdKey] Failed to find Referred SemanticId Builder for Referred SemanticID with Database ID '%d' and Key Database id '%d'", referredSemanticIdDbId, key_id)
		return common.NewInternalServerError("Error during ReferredSemanticId creation. See console for details.")
	}
	return nil
}

// BuildNestedStructure constructs the hierarchical tree of ReferredSemanticIds by linking
// child references to their parent references. This method should be called after all
// references and keys have been added through CreateReferredSemanticId and
// CreateReferredSemanticIdKey.
//
// The method iterates through all ReferredSemanticIds and assigns each one to its parent's
// ReferredSemanticId field, building the complete nested structure. References already
// attached to the root are skipped.
//
// Typical usage pattern:
//
//	// 1. Create the builder
//	ref, builder := NewReferenceBuilder("ExternalReference", 123)
//
//	// 2. Add keys and ReferredSemanticIds (typically in a loop over database rows)
//	builder.CreateKey(1, "Submodel", "https://example.com/submodel")
//	builder.CreateReferredSemanticId(456, 123, "ModelReference")
//	builder.CreateReferredSemanticIdKey(456, 2, "ConceptDescription", "0173-1#01-ABC123#001")
//	builder.CreateReferredSemanticId(789, 456, "ExternalReference")
//	builder.CreateReferredSemanticIdKey(789, 3, "GlobalReference", "https://example.com/concept")
//
//	// 3. Build the nested structure
//	builder.BuildNestedStructure()
//
//	// Now 'ref' contains the complete nested hierarchy
func (rb *ReferenceBuilder) BuildNestedStructure() {
	for _, refMetadata := range rb.referredSemanticIdMap {
		if refMetadata.parent == rb.databaseId {
			// Already assigned to root, skip
			continue
		}
		parentId := refMetadata.parent
		reference := refMetadata.reference
		parentObj := rb.referredSemanticIdMap[parentId].reference
		parentObj.ReferredSemanticId = reference
	}
}
