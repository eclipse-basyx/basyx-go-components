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

package builder

import (
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// MultiReferenceBuilder manages the construction of multiple Reference objects simultaneously
// from flattened database rows. It is used for AAS elements that contain collections of
// references, such as SupplementalSemanticIds.
//
// Each reference in the collection can have its own keys and nested ReferredSemanticId
// structures. The builder internally uses [ReferenceBuilder] instances to construct each
// individual reference, providing a unified interface for managing the entire collection.
//
// The builder tracks database IDs to map database rows to the correct reference objects
// and delegates complex operations like nested structure building to the underlying
// [ReferenceBuilder] instances.
//
// See also: [ReferenceBuilder] for single reference construction.
type MultiReferenceBuilder struct {
	referenceBuilderMap map[int64]*ReferenceBuilder // Maps database IDs to individual reference builders
	references          []*gen.Reference            // Collection of all references being built
}

// NewMultiReferenceBuilder creates a new MultiReferenceBuilder instance and initializes
// an empty slice for collecting multiple Reference objects.
//
// Returns:
//   - []*gen.Reference: A slice of references that will be populated as references are created
//   - *MultiReferenceBuilder: A pointer to the builder for constructing the reference collection
//
// The returned slice can be assigned to an AAS element's field (e.g., SupplementalSemanticIds),
// and as the builder processes database rows, it will automatically populate the slice with
// fully constructed Reference objects.
//
// Example:
//
//	supplementalSemanticIds, builder := NewMultiReferenceBuilder()
//	submodel.SupplementalSemanticIds = supplementalSemanticIds
//	// Later, when processing database rows:
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateKey(101, 1, "GlobalReference", "https://example.com/concept1")
//	builder.CreateReference(102, "ModelReference")
//	builder.CreateKey(102, 2, "ConceptDescription", "0173-1#01-ABC123#001")
//	// Now supplementalSemanticIds contains both references
func NewMultiReferenceBuilder() ([]*gen.Reference, *MultiReferenceBuilder) {
	referenceMap := make(map[int64]*ReferenceBuilder)
	references := []*gen.Reference{}
	return references, &MultiReferenceBuilder{referenceBuilderMap: referenceMap, references: references}
}

// CreateReference creates a new Reference object in the collection with the specified type
// and database ID. This method internally creates a [ReferenceBuilder] for the new reference.
//
// Parameters:
//   - dbId: The database ID of the reference for tracking and duplicate detection
//   - referenceType: The type of reference (e.g., "ExternalReference", "ModelReference")
//
// If a reference with the given dbId already exists, this method does nothing, preventing
// duplicates when processing multiple database rows with the same reference.
//
// See also: [NewReferenceBuilder] for the underlying reference creation.
//
// Example:
//
//	builder := NewMultiReferenceBuilder()
//	// Create multiple references
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateReference(102, "ModelReference")
//	// Calling with same dbId again has no effect
//	builder.CreateReference(101, "ExternalReference") // No-op
func (rb *MultiReferenceBuilder) CreateReference(dbId int64, referenceType string) {
	// Use ReferenceBuilder to create a new reference for MultiReferenceBuilder
	_, exists := rb.referenceBuilderMap[dbId]
	if exists {
		// Reference already exists, do not create a new one
		return
	}
	reference, builder := NewReferenceBuilder(referenceType, dbId)
	rb.referenceBuilderMap[dbId] = builder
	rb.references = append(rb.references, reference)
}

// CreateKey adds a new key to a specific reference in the collection. Keys are the building
// blocks of a reference and define the path to the referenced element.
//
// Parameters:
//   - dbId: The database ID of the reference to add the key to
//   - key_id: The database ID of the key for duplicate detection
//   - key_type: The type of key (e.g., "Submodel", "GlobalReference", "ConceptDescription")
//   - key_value: The value of the key (e.g., a URL or identifier)
//
// This method must be called after [CreateReference] has been called for the corresponding
// dbId. If the reference doesn't exist, the method silently does nothing.
//
// See also: [ReferenceBuilder.CreateKey] for the underlying key creation logic.
//
// Example:
//
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateKey(101, 1, "GlobalReference", "https://example.com/concept")
//	builder.CreateKey(101, 2, "Property", "temperature")
func (rb *MultiReferenceBuilder) CreateKey(dbId int64, key_id int64, key_type string, key_value string) {
	referenceBuilder, exists := rb.referenceBuilderMap[dbId]
	if exists {
		referenceBuilder.CreateKey(key_id, key_type, key_value)
	}
}

// CreateReferredSemanticId creates a new ReferredSemanticId within a specific reference in
// the collection. ReferredSemanticIds can be nested, forming a tree structure where each
// reference can have its own ReferredSemanticId.
//
// Parameters:
//   - rootSemanticIdDbId: The database ID of the root reference to add the ReferredSemanticId to
//   - referredSemanticIdDbId: The database ID of the ReferredSemanticId reference
//   - referredSemanticIdParentId: The database ID of the parent reference in the hierarchy
//   - referredSemanticIdType: The type of the ReferredSemanticId reference
//
// Returns:
//   - error: Returns an error if the root reference builder cannot be found, nil otherwise
//
// This method delegates to the underlying [ReferenceBuilder] for the specified root reference.
// It must be called after [CreateReference] has been called for the rootSemanticIdDbId.
//
// See also: [ReferenceBuilder.CreateReferredSemanticId] for the underlying implementation.
//
// Example:
//
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateKey(101, 1, "GlobalReference", "https://example.com/concept")
//	// Add a ReferredSemanticId to reference 101
//	err := builder.CreateReferredSemanticId(101, 201, 101, "ModelReference")
//	if err != nil {
//	    // Handle error
//	}
func (rb *MultiReferenceBuilder) CreateReferredSemanticId(rootSemanticIdDbId int64, referredSemanticIdDbId int64, referredSemanticIdParentId int64, referredSemanticIdType string) error {
	referenceBuilder, exists := rb.referenceBuilderMap[rootSemanticIdDbId]
	if exists {
		referenceBuilder.CreateReferredSemanticId(referredSemanticIdDbId, referredSemanticIdParentId, referredSemanticIdType)
	} else {
		fmt.Printf("[MultiReferenceBuilder:CreateReferredSemanticId] Failed to find Referred SemanticId Builder for Referred SemanticID with Database ID '%d'", referredSemanticIdDbId)
		return common.NewInternalServerError("Error during ReferredSemanticId creation. See console for details.")
	}
	return nil
}

// CreateReferredSemanticIdKey adds a key to a specific ReferredSemanticId within a reference
// in the collection. This method delegates to the underlying [ReferenceBuilder].
//
// Parameters:
//   - rootSemanticIdDbId: The database ID of the root reference containing the ReferredSemanticId
//   - referredSemanticIdDbId: The database ID of the ReferredSemanticId to add the key to
//   - referredSemanticIdKeyDbId: The database ID of the key for duplicate detection
//   - referredSemanticIdKeyType: The type of key (e.g., "ConceptDescription", "GlobalReference")
//   - referredSemanticIdKeyValue: The value of the key
//
// Returns:
//   - error: Returns an error if the root reference builder cannot be found, nil otherwise
//
// This method must be called after both [CreateReference] and [CreateReferredSemanticId] have
// been called for the corresponding IDs.
//
// See also: [ReferenceBuilder.CreateReferredSemanticIdKey] for the underlying implementation.
//
// Example:
//
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateReferredSemanticId(101, 201, 101, "ModelReference")
//	// Add a key to the ReferredSemanticId
//	err := builder.CreateReferredSemanticIdKey(101, 201, 1, "ConceptDescription", "0173-1#01-ABC123#001")
//	if err != nil {
//	    // Handle error
//	}
func (rb *MultiReferenceBuilder) CreateReferredSemanticIdKey(rootSemanticIdDbId int64, referredSemanticIdDbId int64, referredSemanticIdKeyDbId int64, referredSemanticIdKeyType string, referredSemanticIdKeyValue string) error {
	referenceBuilder, exists := rb.referenceBuilderMap[rootSemanticIdDbId]
	if exists {
		referenceBuilder.CreateReferredSemanticIdKey(referredSemanticIdDbId, referredSemanticIdKeyDbId, referredSemanticIdKeyType, referredSemanticIdKeyValue)
	} else {
		fmt.Printf("[MultiReferenceBuilder:CreateReferredSemanticIdKey] Failed to find Referred SemanticId Builder for Referred SemanticID with Database ID '%d' and Key Database id '%d'", referredSemanticIdDbId, referredSemanticIdKeyDbId)
		return common.NewInternalServerError("Error during ReferredSemanticId creation. See console for details.")
	}
	return nil
}

// BuildNestedStructures constructs the hierarchical trees of ReferredSemanticIds for all
// references in the collection. This method should be called after all references, keys,
// and ReferredSemanticIds have been added.
//
// The method delegates to [ReferenceBuilder.BuildNestedStructure] on each underlying [ReferenceBuilder],
// which links child references to their parent references, building the complete nested
// structure for each reference in the collection.
//
// Typical usage pattern:
//
//	// 1. Create the builder
//	supplementalSemanticIds, builder := NewMultiReferenceBuilder()
//
//	// 2. Add references, keys, and ReferredSemanticIds (typically in a loop over database rows)
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateKey(101, 1, "GlobalReference", "https://example.com/concept1")
//	builder.CreateReferredSemanticId(101, 201, 101, "ModelReference")
//	builder.CreateReferredSemanticIdKey(101, 201, 2, "ConceptDescription", "0173-1#01-ABC123#001")
//
//	builder.CreateReference(102, "ModelReference")
//	builder.CreateKey(102, 3, "Submodel", "https://example.com/submodel")
//
//	// 3. Build the nested structures for all references
//	builder.BuildNestedStructures()
//
//	// Now supplementalSemanticIds contains all references with complete nested hierarchies
func (rb *MultiReferenceBuilder) BuildNestedStructures() {
	for _, builders := range rb.referenceBuilderMap {
		builders.BuildNestedStructure()
	}
}

// GetReferences returns the collection of all references that have been built. This method
// can be called at any time to retrieve the current state of the reference collection.
//
// Returns:
//   - []*gen.Reference: A slice of all Reference objects in the collection
//
// Note: The references are returned in the order they were created. If [BuildNestedStructures]
// has not been called yet, the ReferredSemanticId hierarchies may not be fully linked.
//
// Example:
//
//	builder := NewMultiReferenceBuilder()
//	builder.CreateReference(101, "ExternalReference")
//	builder.CreateKey(101, 1, "GlobalReference", "https://example.com/concept")
//	builder.BuildNestedStructures()
//	references := builder.GetReferences()
//	// Process the references
func (rb *MultiReferenceBuilder) GetReferences() []*gen.Reference {
	return rb.references
}

// GetReferenceBuilder returns the underlying [ReferenceBuilder] for a specific reference
// in the collection. This provides direct access to the builder for advanced operations.
//
// Parameters:
//   - dbId: The database ID of the reference whose builder should be retrieved
//
// Returns:
//   - *ReferenceBuilder: The [ReferenceBuilder] for the specified reference, or nil if not found
//
// This method is useful when you need to perform operations directly on a specific
// [ReferenceBuilder], such as calling methods that aren't exposed through the
// [MultiReferenceBuilder] interface.
//
// Example:
//
//	builder := NewMultiReferenceBuilder()
//	builder.CreateReference(101, "ExternalReference")
//	refBuilder := builder.GetReferenceBuilder(101)
//	if refBuilder != nil {
//	    // Perform advanced operations on the specific builder
//	}
func (rb *MultiReferenceBuilder) GetReferenceBuilder(dbId int64) *ReferenceBuilder {
	return rb.referenceBuilderMap[dbId]
}
