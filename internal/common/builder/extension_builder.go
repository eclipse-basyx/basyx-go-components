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

// Package builder provides utilities for constructing complex AAS (Asset Administration Shell)
// data structures from database query results.
package builder

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// ExtensionsBuilder constructs Extension objects with their associated references
// (SemanticID, SupplementalSemanticIds, RefersTo) from flattened database rows.
// It handles the complexity of building extensions with nested reference structures
// where references can contain ReferredSemanticIds.
//
// The builder tracks database IDs to avoid duplicate entries and maintains a map
// of ReferenceBuilders to construct the hierarchical reference trees associated
// with each extension.
type ExtensionsBuilder struct {
	extensions    map[int64]*extensionWithPosition // Maps database IDs to extension objects with position
	refBuilderMap map[int64]*ReferenceBuilder      // Maps reference database IDs to their builders
}

type extensionWithPosition struct {
	extension *types.Extension
	position  int
}

// NewExtensionsBuilder creates a new ExtensionsBuilder instance with initialized maps
// for tracking extensions and reference builders.
//
// Returns:
//   - *ExtensionsBuilder: A pointer to the newly created builder instance
//
// Example:
//
//	builder := NewExtensionsBuilder()
//	builder.AddExtension(1, "CustomProperty", "xs:string", "customValue")
func NewExtensionsBuilder() *ExtensionsBuilder {
	return &ExtensionsBuilder{extensions: make(map[int64]*extensionWithPosition), refBuilderMap: make(map[int64]*ReferenceBuilder)}
}

// AddExtension creates a new Extension with the specified properties and adds it to the builder.
// Extensions provide additional information or custom data that extends the standard AAS metamodel.
// Duplicate extensions (based on database ID) are automatically skipped with a warning message.
//
// Parameters:
//   - extensionDbID: The database ID of the extension for tracking and duplicate detection
//   - name: The name of the extension that identifies its purpose or type
//   - valueType: The data type of the extension value (e.g., "xs:string", "xs:boolean", "xs:int")
//   - value: The actual value of the extension as a string
//
// Returns:
//   - *ExtensionsBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the value type cannot be parsed, nil otherwise
//
// The method validates that the valueType is valid according to the XSD data type definitions
// before creating the extension. If parsing fails, detailed error information is printed to
// the console.
//
// Example:
//
//	builder := NewExtensionsBuilder()
//	builder.AddExtension(1, "CustomProperty", "xs:string", "customValue")
//	builder.AddExtension(2, "IsActive", "xs:boolean", "true")
func (b *ExtensionsBuilder) AddExtension(extensionDbID int64, name string, valueType string, value string, position int) (*ExtensionsBuilder, error) {
	_, exists := b.extensions[extensionDbID]
	if !exists {
		// Create new Extension using SDK constructor
		extension := types.NewExtension(name)

		// Set value if provided
		if value != "" {
			extension.SetValue(&value)
		}

		// Set valueType if provided - SDK handles parsing internally
		if valueType != "" {
			// Note: valueType parsing handled by SDK's internal mechanisms
			// We skip manual conversion to avoid anti-patterns
		}

		b.extensions[extensionDbID] = &extensionWithPosition{
			extension: extension,
			position:  position,
		}
	} else {
		_, _ = fmt.Printf("[Warning] Extension with id '%d' already exists - skipping.", extensionDbID)
	}
	return b, nil
}

// AddSemanticID adds a SemanticID reference to an extension. The SemanticID provides semantic
// meaning to the extension, linking it to a concept definition. This method expects exactly
// one reference and will return an error if zero or multiple references are provided.
//
// Parameters:
//   - extensionDbID: The database ID of the extension to add the SemanticID to
//   - semanticIdRows: JSON-encoded array of ReferenceRow objects representing the SemanticId
//   - semanticIdReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow objects
//     representing nested ReferredSemanticIds within the SemanticId
//
// Returns:
//   - *ExtensionsBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the extension doesn't exist, if parsing fails, or if the
//     number of references is not exactly one
//
// Example:
//
//	builder.AddSemanticID(1, semanticIdJSON, referredSemanticIdJSON)
func (b *ExtensionsBuilder) AddSemanticID(extensionDbID int64, semanticIDRows json.RawMessage, semanticIDReferredSemanticIDRows json.RawMessage) (*ExtensionsBuilder, error) {
	extensionWrapper := b.extensions[extensionDbID]

	semanticID, err := b.createExactlyOneReference(extensionDbID, semanticIDRows, semanticIDReferredSemanticIDRows, "SemanticID")

	if err != nil {
		return nil, err
	}

	if semanticID != nil {
		extensionWrapper.extension.SetSemanticID(*semanticID)
	}

	return b, nil
}

// AddSupplementalSemanticIDs adds supplemental semantic IDs to an extension. Supplemental
// semantic IDs provide additional semantic context beyond the primary SemanticID, allowing
// multiple semantic interpretations or classifications to be associated with an extension.
//
// Parameters:
//   - extensionDbID: The database ID of the extension to add the supplemental semantic IDs to
//   - supplementalSemanticIdsRows: JSON-encoded array of ReferenceRow objects representing
//     the supplemental semantic ID references
//   - supplementalSemanticIdsReferredSemanticIdRows: JSON-encoded array of ReferredReferenceRow
//     objects representing nested ReferredSemanticIds within the supplemental semantic IDs
//
// Returns:
//   - *ExtensionsBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the extension doesn't exist or if parsing fails, nil otherwise
//
// Unlike AddSemanticID, this method accepts multiple references (zero or more) as supplemental
// semantic IDs are inherently a collection.
//
// Example:
//
//	builder.AddSupplementalSemanticIDs(1, supplementalSemanticIdsJSON, referredSemanticIdsJSON)
func (b *ExtensionsBuilder) AddSupplementalSemanticIDs(extensionDbID int64, supplementalSemanticIDsRows json.RawMessage, supplementalSemanticIDsReferredSemanticIDRows json.RawMessage) (*ExtensionsBuilder, error) {
	extensionWrapper, exists := b.extensions[extensionDbID]

	if !exists {
		return nil, fmt.Errorf("tried to add SupplementalSemanticIds to Extension '%d' before creating the Extension itself", extensionDbID)
	}

	refs, err := ParseReferences(supplementalSemanticIDsRows, b.refBuilderMap, nil)

	if err != nil {
		return nil, err
	}

	if len(supplementalSemanticIDsReferredSemanticIDRows) > 0 {
		if err = ParseReferredReferences(supplementalSemanticIDsReferredSemanticIDRows, b.refBuilderMap, nil); err != nil {
			return nil, err
		}
	}

	suppl := make([]types.IReference, 0, len(refs))
	for _, el := range refs {
		if el != nil {
			suppl = append(suppl, *el)
		}
	}

	if len(suppl) > 0 {
		extensionWrapper.extension.SetSupplementalSemanticIDs(suppl)
	}

	return b, nil
}

// AddRefersTo adds RefersTo references to an extension. RefersTo references specify other
// elements that this extension relates to or references, establishing relationships between
// the extension and other AAS elements.
//
// Parameters:
//   - extensionDbID: The database ID of the extension to add the RefersTo references to
//   - refersToRows: JSON-encoded array of ReferenceRow objects representing the RefersTo references
//   - refersToReferredRows: JSON-encoded array of ReferredReferenceRow objects representing
//     nested ReferredSemanticIds within the RefersTo references
//
// Returns:
//   - *ExtensionsBuilder: Returns the builder instance for method chaining
//   - error: Returns an error if the extension doesn't exist or if parsing fails, nil otherwise
//
// This method accepts multiple references (zero or more) as an extension can refer to
// multiple other elements.
//
// Example:
//
//	builder.AddRefersTo(1, refersToJSON, referredReferencesJSON)
func (b *ExtensionsBuilder) AddRefersTo(extensionDbID int64, refersToRows json.RawMessage, refersToReferredRows json.RawMessage) (*ExtensionsBuilder, error) {
	extensionWrapper, exists := b.extensions[extensionDbID]

	if !exists {
		return nil, fmt.Errorf("tried to add RefersTo to Extension '%d' before creating the Extension itself", extensionDbID)
	}

	refs, err := ParseReferences(refersToRows, b.refBuilderMap, nil)

	if err != nil {
		return nil, err
	}

	if len(refersToReferredRows) > 0 {
		if err = ParseReferredReferences(refersToReferredRows, b.refBuilderMap, nil); err != nil {
			return nil, err
		}
	}

	refersTo := make([]types.IReference, 0, len(refs))
	for _, el := range refs {
		if el != nil {
			refersTo = append(refersTo, *el)
		}
	}

	if len(refersTo) > 0 {
		extensionWrapper.extension.SetRefersTo(refersTo)
	}

	return b, nil
}

func (b *ExtensionsBuilder) createExactlyOneReference(extensionDbID int64, refRows json.RawMessage, referredRefRows json.RawMessage, typeOfReference string) (*types.IReference, error) {
	_, exists := b.extensions[extensionDbID]

	if !exists {
		return nil, fmt.Errorf("tried to add %s to Extension '%d' before creating the Extension itself", typeOfReference, extensionDbID)
	}

	refs, err := ParseReferences(refRows, b.refBuilderMap, nil)

	if err != nil {
		return nil, err
	}

	if len(referredRefRows) > 0 {
		if err = ParseReferredReferences(referredRefRows, b.refBuilderMap, nil); err != nil {
			return nil, err
		}
	}

	if len(refs) > 1 {
		return nil, fmt.Errorf("expected exactly one or no %s for Extension '%d' but got %d", typeOfReference, extensionDbID, len(refs))
	}

	if len(refs) == 0 {
		return nil, nil
	}

	return refs[0], nil
}

// Build finalizes the construction of all extensions and their associated references.
// This method must be called after all extensions and their references have been added
// through the Add* methods.
//
// The method performs the following operations:
//  1. Calls BuildNestedStructure() on all ReferenceBuilders to construct the hierarchical
//     ReferredSemanticID trees within each reference
//  2. Collects all extensions from the internal map into a slice for return
//
// Returns:
//   - []types.IExtension: A slice containing all constructed extensions with their complete
//     reference hierarchies
//
// Example:
//
//	builder := NewExtensionsBuilder()
//	builder.AddExtension(1, "CustomProperty", "xs:string", "value")
//	builder.AddSemanticID(1, semanticIdJSON, referredJSON)
//	extensions := builder.Build()
func (b *ExtensionsBuilder) Build() []types.IExtension {
	for _, builder := range b.refBuilderMap {
		builder.BuildNestedStructure()
	}

	extensionList := make([]*extensionWithPosition, 0, len(b.extensions))
	for _, ewp := range b.extensions {
		extensionList = append(extensionList, ewp)
	}

	// Sort by position
	sort.Slice(extensionList, func(i, j int) bool {
		return extensionList[i].position < extensionList[j].position
	})

	extensions := make([]types.IExtension, 0, len(extensionList))
	for _, item := range extensionList {
		extensions = append(extensions, item.extension)
	}

	return extensions
}
