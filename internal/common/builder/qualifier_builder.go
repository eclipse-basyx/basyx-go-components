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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package builder

import (
	"encoding/json"
	"fmt"
	"sort"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// QualifiersBuilder constructs Qualifier objects with their associated references
// (SemanticID, ValueID, SupplementalSemanticIds) from flattened database rows.
// It handles the complexity of building qualifiers with nested reference structures
// where references can contain ReferredSemanticIds.
//
// The builder tracks database IDs to avoid duplicate entries and maintains a map
// of ReferenceBuilders to construct the hierarchical reference trees associated
// with each qualifier.
type QualifiersBuilder struct {
	qualifiers    map[int64]*qualifierWithPosition
	refBuilderMap map[int64]*ReferenceBuilder
}

type qualifierWithPosition struct {
	qualifier *gen.Qualifier
	position  int
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
	return &QualifiersBuilder{qualifiers: make(map[int64]*qualifierWithPosition), refBuilderMap: make(map[int64]*ReferenceBuilder)}
}

// AddQualifier creates a new Qualifier with the specified properties and adds it to the builder.
// Qualifiers provide additional information about other AAS elements and can restrict their
// values or semantics. Duplicate qualifiers (based on database ID) are automatically skipped
// with a warning message.
//
// Parameters:
//   - qualifierDbID: The database ID of the qualifier for tracking and duplicate detection
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
func (b *QualifiersBuilder) AddQualifier(qualifierDbID int64, kind string, qType string, valueType string, value string, position int) (*QualifiersBuilder, error) {
	_, exists := b.qualifiers[qualifierDbID]
	if !exists {
		ValueType, err := gen.NewDataTypeDefXsdFromValue(valueType)
		if err != nil {
			fmt.Println(err)
			return nil, fmt.Errorf("error parsing ValueType for Qualifier '%d' to Go Struct. See console for details", qualifierDbID)
		}
		b.qualifiers[qualifierDbID] = &qualifierWithPosition{
			qualifier: &gen.Qualifier{
				Type:      qType,
				ValueType: ValueType,
				Value:     value,
			},
			position: position,
		}

		if kind != "" {
			Kind, err := gen.NewQualifierKindFromValue(kind)
			if err != nil {
				fmt.Println(err)
				return nil, fmt.Errorf("error parsing Qualifier Kind to Go Struct for Qualifier '%d'. See console for details", qualifierDbID)
			}
			b.qualifiers[qualifierDbID].qualifier.Kind = Kind
		}
	} else {
		fmt.Printf("[Warning] qualifier with id '%d' already exists - skipping.", qualifierDbID)
	}

	return b, nil
}

// AddSemanticID adds a SemanticID reference to a qualifier. This method expects exactly one reference and will return an error if zero or
// multiple references are provided.
//
// Parameters:
//   - qualifierDbID: The database ID of the qualifier to add the SemanticID to
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
//	builder.AddSemanticID(1, semanticIdJSON, referredSemanticIdJSON)
func (b *QualifiersBuilder) AddSemanticID(qualifierDbID int64, semanticIDRows json.RawMessage, semanticIDReferredSemanticIDRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbID]

	semanticID, err := b.createExactlyOneReference(qualifierDbID, semanticIDRows, semanticIDReferredSemanticIDRows, "SemanticID")

	if err != nil {
		return nil, err
	}

	qualifier.qualifier.SemanticID = semanticID

	return b, nil
}

// AddValueID adds a ValueID reference to a qualifier. The ValueID references the value
// of the qualifier in a global, unique way, allowing the qualifier's value to be
// semantically interpreted across different contexts. This method expects exactly one
// reference and will return an error if zero or multiple references are provided.
//
// Parameters:
//   - qualifierDbID: The database ID of the qualifier to add the ValueID to
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
//	builder.AddValueID(1, valueIdJSON, referredSemanticIdJSON)
func (b *QualifiersBuilder) AddValueID(qualifierDbID int64, valueIDRows json.RawMessage, valueIDReferredSemanticIDRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier := b.qualifiers[qualifierDbID]

	valueID, err := b.createExactlyOneReference(qualifierDbID, valueIDRows, valueIDReferredSemanticIDRows, "ValueId")

	if err != nil {
		return nil, err
	}

	qualifier.qualifier.ValueID = valueID
	return b, nil
}

// AddSupplementalSemanticIDs adds supplemental semantic IDs to a qualifier. Supplemental
// semantic IDs provide additional semantic context beyond the primary SemanticID, allowing
// multiple semantic interpretations or classifications to be associated with a qualifier.
//
// Parameters:
//   - qualifierDbID: The database ID of the qualifier to add the supplemental semantic IDs to
//   - supplementalSemanticIdsRows: JSON-encoded array of ReferenceRow objects representing
//     the supplemental semantic ID references
//   - supplementalSemanticIdsReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow
//     objects representing nested ReferredSemanticIds within the supplemental semantic IDs
//
// Returns:
//   - *QualifiersBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the qualifier doesn't exist or if parsing fails, nil otherwise
//
// Unlike AddSemanticID and AddValueID, this method accepts multiple references (zero or more)
// as supplemental semantic IDs are inherently a collection. Each reference can have its own
// nested ReferredSemanticID hierarchy.
//
// Example:
//
//	builder.AddSupplementalSemanticIDs(1, supplementalSemanticIdsJSON, referredSemanticIdsJSON)
func (b *QualifiersBuilder) AddSupplementalSemanticIDs(qualifierDbID int64, supplementalSemanticIDsRows json.RawMessage, supplementalSemanticIDsReferredSemanticIDRows json.RawMessage) (*QualifiersBuilder, error) {
	qualifier, exists := b.qualifiers[qualifierDbID]

	if !exists {
		return nil, fmt.Errorf("tried to add SupplementalSemanticIds to Qualifier '%d' before creating the Qualifier itself", qualifierDbID)
	}

	refs, err := ParseReferences(supplementalSemanticIDsRows, b.refBuilderMap, nil)

	if err != nil {
		return nil, err
	}

	if len(supplementalSemanticIDsReferredSemanticIDRows) > 0 {
		err = ParseReferredReferences(supplementalSemanticIDsReferredSemanticIDRows, b.refBuilderMap, nil)
		if err != nil {
			return nil, err
		}
	}

	suppl := []gen.Reference{}

	for _, el := range refs {
		suppl = append(suppl, *el)
	}

	qualifier.qualifier.SupplementalSemanticIds = suppl

	return b, nil
}

func (b *QualifiersBuilder) createExactlyOneReference(qualifierDbID int64, refRows json.RawMessage, referredRefRows json.RawMessage, typeOfReference string) (*gen.Reference, error) {
	_, exists := b.qualifiers[qualifierDbID]

	if !exists {
		return nil, fmt.Errorf("tried to add %s to Qualifier '%d' before creating the Qualifier itself", typeOfReference, qualifierDbID)
	}

	refs, err := ParseReferences(refRows, b.refBuilderMap, nil)

	if err != nil {
		return nil, err
	}

	if len(referredRefRows) > 0 {
		err = ParseReferredReferences(referredRefRows, b.refBuilderMap, nil)
		if err != nil {
			return nil, err
		}
	}

	if len(refs) > 1 {
		return nil, fmt.Errorf("expected exactly one or no %s for Qualifier '%d' but got %d", typeOfReference, qualifierDbID, len(refs))
	}

	if len(refs) == 0 {
		return nil, nil
	}

	return refs[0], nil
}

// Build finalizes the construction of all qualifiers and their associated references.
// This method must be called after all qualifiers and their references have been added
// through the Add* methods. It performs the following operations:
//
//  1. Calls BuildNestedStructure() on all ReferenceBuilders to construct the hierarchical
//     ReferredSemanticID trees within each reference
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
//	builder.AddSemanticID(1, semanticIdRows, referredSemanticIdRows)
//	builder.AddValueID(1, valueIdRows, referredValueIdRows)
//	builder.AddSupplementalSemanticIds(1, supplSemanticIdsRows, supplReferredRows)
//
//	builder.AddQualifier(2, "ValueQualifier", "ExpressionLogic", "xs:boolean", "true")
//	builder.AddValueID(2, valueIdRows2, referredValueIdRows2)
//
//	// 3. Build and retrieve the final qualifiers
//	qualifiers := builder.Build()
//
//	// Now 'qualifiers' contains all qualifiers with complete reference hierarchies
func (b *QualifiersBuilder) Build() []gen.Qualifier {
	for _, builder := range b.refBuilderMap {
		builder.BuildNestedStructure()
	}

	qualifierList := make([]*qualifierWithPosition, 0, len(b.qualifiers))
	for _, qwp := range b.qualifiers {
		qualifierList = append(qualifierList, qwp)
	}

	// Sort by position
	sort.Slice(qualifierList, func(i, j int) bool {
		return qualifierList[i].position < qualifierList[j].position
	})

	qualifiers := make([]gen.Qualifier, 0, len(qualifierList))
	for _, item := range qualifierList {
		qualifiers = append(qualifiers, *item.qualifier)
	}

	return qualifiers
}
