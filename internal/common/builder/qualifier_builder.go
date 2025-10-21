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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package builder

import (
	"encoding/json"
	"fmt"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// QualifiersBuilder constructs Qualifier objects with their associated references
// (SemanticId, ValueId, SupplementalSemanticIds) from flattened database rows.
// It handles the complexity of building qualifiers with nested reference structures
// where references can contain ReferredSemanticIds.
//
// The builder tracks database IDs to avoid duplicate entries and maintains a map
// of ReferenceBuilders to construct the hierarchical reference trees associated
// with each qualifier.
type QualifiersBuilder struct {
	qualifiers    map[int64]*gen.Qualifier    // Maps database IDs to qualifier objects
	refBuilderMap map[int64]*ReferenceBuilder // Maps reference database IDs to their builders
}

// NewQualifiersBuilder creates a new QualifiersBuilder instance with initialized maps
// for tracking qualifiers and reference builders.
//
// Returns:
//   - *QualifiersBuilder: A pointer to the newly created builder instance
//
// Example:
//
//	builder := NewQualifiersBuilder()
//	builder.AddQualifier(1, "ConceptQualifier", "ExpressionSemantic", "xs:string", "example value")
func NewQualifiersBuilder() *QualifiersBuilder {
	return &QualifiersBuilder{qualifiers: make(map[int64]*gen.Qualifier), refBuilderMap: make(map[int64]*ReferenceBuilder)}
}

// AddQualifier creates a new Qualifier with the specified properties and adds it to the builder.
// Qualifiers provide additional information about other AAS elements and can restrict their
// values or semantics. Duplicate qualifiers (based on database ID) are automatically skipped
// with a warning message.
//
// Parameters:
//   - qualifierDbId: The database ID of the qualifier for tracking and duplicate detection
//   - kind: The kind of qualifier (e.g., "ConceptQualifier", "ValueQualifier", "TemplateQualifier")
//   - qType: The type that qualifies the qualifier itself (semantic identifier)
//   - valueType: The data type of the qualifier value (e.g., "xs:string", "xs:boolean", "xs:int")
//   - value: The actual value of the qualifier as a string
//
// Returns:
//   - *QualifiersBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the qualifier kind or value type cannot be parsed, nil otherwise
//
// The method validates that the kind and valueType are valid according to the AAS metamodel
// before creating the qualifier. If parsing fails, detailed error information is printed to
// the console.
//
// Example:
//
//	builder := NewQualifiersBuilder()
//	builder.AddQualifier(1, "ConceptQualifier", "ExpressionSemantic", "xs:string", "example value")
//	builder.AddQualifier(2, "ValueQualifier", "ExpressionLogic", "xs:boolean", "true")
func (b *QualifiersBuilder) AddQualifier(qualifierDbId int64, kind string, qType string, valueType string, value string) (*QualifiersBuilder, error) {
	_, exists := b.qualifiers[qualifierDbId]
	if !exists {
		Kind, err := gen.NewQualifierKindFromValue(kind)
		if err != nil {
			fmt.Println(err)
			return nil, fmt.Errorf("error parsing Qualifier Kind to Go Struct for Qualifier '%d'. See console for details", qualifierDbId)
		}
		ValueType, err := gen.NewDataTypeDefXsdFromValue(valueType)
		if err != nil {
			fmt.Println(err)
			return nil, fmt.Errorf("error parsing ValueType for Qualifier '%d' to Go Struct. See console for details", qualifierDbId)
		}
		b.qualifiers[qualifierDbId] = &gen.Qualifier{
			Kind:      Kind,
			Type:      qType,
			ValueType: ValueType,
			Value:     value,
		}
	} else {
		fmt.Printf("[Warning] qualifier with id '%d' already exists - skipping.", qualifierDbId)
	}
	return b, nil
}

// AddSemanticId adds a SemanticId reference to a qualifier. This method expects exactly one reference and will return an error if zero or
// multiple references are provided.
//
// Parameters:
//   - qualifierDbId: The database ID of the qualifier to add the SemanticId to
//   - semanticIdRows: JSON-encoded array of ReferenceRow objects representing the SemanticId
//   - semanticIdReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow objects
//     representing nested ReferredSemanticIds within the SemanticId
//
// Returns:
//   - *QualifiersBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the qualifier doesn't exist, if parsing fails, or if the
//     number of references is not exactly one
//
// The method uses the internal createExactlyOneReference helper to ensure exactly one
// reference is created from the provided rows. It also processes any nested ReferredSemanticIds
// to build the complete reference hierarchy.
//
// Example:
//
//	builder.AddSemanticId(1, semanticIdJSON, referredSemanticIdJSON)
func (b *QualifiersBuilder) AddSemanticId(qualifierDbId int64, semanticIdRows json.RawMessage, semanticIdReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbId]

	semanticId, err := b.createExactlyOneReference(qualifierDbId, semanticIdRows, semanticIdReferredSemanticIdRows, "SemanticID")

	if err != nil {
		return nil, err
	}

	qualifier.SemanticId = semanticId

	return b, nil
}

// AddValueId adds a ValueId reference to a qualifier. The ValueId references the value
// of the qualifier in a global, unique way, allowing the qualifier's value to be
// semantically interpreted across different contexts. This method expects exactly one
// reference and will return an error if zero or multiple references are provided.
//
// Parameters:
//   - qualifierDbId: The database ID of the qualifier to add the ValueId to
//   - valueIdRows: JSON-encoded array of ReferenceRow objects representing the ValueId
//   - valueIdReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow objects
//     representing nested ReferredSemanticIds within the ValueId
//
// Returns:
//   - *QualifiersBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the qualifier doesn't exist, if parsing fails, or if the
//     number of references is not exactly one
//
// The method uses the internal createExactlyOneReference helper to ensure exactly one
// reference is created from the provided rows. It also processes any nested ReferredSemanticIds
// to build the complete reference hierarchy.
//
// Example:
//
//	builder.AddValueId(1, valueIdJSON, referredSemanticIdJSON)
func (b *QualifiersBuilder) AddValueId(qualifierDbId int64, valueIdRows json.RawMessage, valueIdReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbId]

	valueId, err := b.createExactlyOneReference(qualifierDbId, valueIdRows, valueIdReferredSemanticIdRows, "ValueId")

	if err != nil {
		return nil, err
	}

	qualifier.ValueId = valueId
	return b, nil
}

// AddSupplementalSemanticIds adds supplemental semantic IDs to a qualifier. Supplemental
// semantic IDs provide additional semantic context beyond the primary SemanticId, allowing
// multiple semantic interpretations or classifications to be associated with a qualifier.
//
// Parameters:
//   - qualifierDbId: The database ID of the qualifier to add the supplemental semantic IDs to
//   - supplementalSemanticIdsRows: JSON-encoded array of ReferenceRow objects representing
//     the supplemental semantic ID references
//   - supplementalSemanticIdsReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow
//     objects representing nested ReferredSemanticIds within the supplemental semantic IDs
//
// Returns:
//   - *QualifiersBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the qualifier doesn't exist or if parsing fails, nil otherwise
//
// Unlike AddSemanticId and AddValueId, this method accepts multiple references (zero or more)
// as supplemental semantic IDs are inherently a collection. Each reference can have its own
// nested ReferredSemanticId hierarchy.
//
// Example:
//
//	builder.AddSupplementalSemanticIds(1, supplementalSemanticIdsJSON, referredSemanticIdsJSON)
func (b *QualifiersBuilder) AddSupplementalSemanticIds(qualifierDbId int64, supplementalSemanticIdsRows json.RawMessage, supplementalSemanticIdsReferredSemanticIdRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier, exists := b.qualifiers[qualifierDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add SupplementalSemanticIds to Qualifier '%d' before creating the Qualifier itself", qualifierDbId)
	}

	refs, err := ParseReferences(supplementalSemanticIdsRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(supplementalSemanticIdsReferredSemanticIdRows) > 0 {
		ParseReferredReferences(supplementalSemanticIdsReferredSemanticIdRows, b.refBuilderMap)
	}

	suppl := []gen.Reference{}

	for _, el := range refs {
		suppl = append(suppl, *el)
	}

	qualifier.SupplementalSemanticIds = suppl

	return b, nil
}

// createExactlyOneReference is an internal helper method that creates exactly one reference
// from JSON-encoded database rows. It's used by AddSemanticId and AddValueId to ensure
// that these single-reference fields have exactly one reference assigned.
//
// Parameters:
//   - qualifierDbId: The database ID of the qualifier being modified
//   - refRows: JSON-encoded array of ReferenceRow objects
//   - referredRefRows: JSON-encoded array of ReferredReferenceRow objects for nested references
//   - Type: A string describing the type of reference being created (e.g., "SemanticID", "ValueId")
//     used for error messages
//
// Returns:
//   - *gen.Reference: The created reference if exactly one reference is found
//   - error: Returns an error if:
//   - The qualifier with qualifierDbId doesn't exist
//   - JSON parsing fails
//   - Zero or more than one reference is found in the data
//
// The method validates that exactly one reference is created because SemanticId and ValueId
// are single-reference fields in the AAS metamodel. It also processes any ReferredSemanticIds
// to build the complete reference hierarchy.
func (b *QualifiersBuilder) createExactlyOneReference(qualifierDbId int64, refRows json.RawMessage, referredRefRows json.RawMessage, Type string) (*gen.Reference, error) {
	_, exists := b.qualifiers[qualifierDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add %s to Qualifier '%d' before creating the Qualifier itself", Type, qualifierDbId)
	}

	refs, err := ParseReferences(refRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(referredRefRows) > 0 {
		ParseReferredReferences(referredRefRows, b.refBuilderMap)
	}

	if len(refs) != 1 {
		return nil, fmt.Errorf("expected exactly one or no %s for Qualifier '%d' but got %d", Type, qualifierDbId, len(refs))
	}

	return refs[0], nil
}

// Build finalizes the construction of all qualifiers and their associated references.
// This method must be called after all qualifiers and their references have been added
// through the Add* methods. It performs the following operations:
//
//  1. Calls BuildNestedStructure() on all ReferenceBuilders to construct the hierarchical
//     ReferredSemanticId trees within each reference
//  2. Collects all qualifiers from the internal map into a slice for return
//
// Returns:
//   - []gen.Qualifier: A slice containing all constructed qualifiers with their complete
//     reference hierarchies
//
// After calling Build(), the builder can be discarded as all data has been extracted
// and properly structured. The returned qualifiers contain fully constructed references
// with nested ReferredSemanticIds where applicable.
//
// Typical usage pattern:
//
//	// 1. Create the builder
//	builder := NewQualifiersBuilder()
//
//	// 2. Add qualifiers and their references (typically in a loop over database rows)
//	builder.AddQualifier(1, "ConceptQualifier", "ExpressionSemantic", "xs:string", "example")
//	builder.AddSemanticId(1, semanticIdRows, referredSemanticIdRows)
//	builder.AddValueId(1, valueIdRows, referredValueIdRows)
//	builder.AddSupplementalSemanticIds(1, supplSemanticIdsRows, supplReferredRows)
//
//	builder.AddQualifier(2, "ValueQualifier", "ExpressionLogic", "xs:boolean", "true")
//	builder.AddValueId(2, valueIdRows2, referredValueIdRows2)
//
//	// 3. Build and retrieve the final qualifiers
//	qualifiers := builder.Build()
//
//	// Now 'qualifiers' contains all qualifiers with complete reference hierarchies
func (b *QualifiersBuilder) Build() []gen.Qualifier {

	for _, builder := range b.refBuilderMap {
		builder.BuildNestedStructure()
	}

	qualifiers := make([]gen.Qualifier, 0, len(b.qualifiers))
	for _, qualifier := range b.qualifiers {
		qualifiers = append(qualifiers, *qualifier)
	}

	return qualifiers
}
