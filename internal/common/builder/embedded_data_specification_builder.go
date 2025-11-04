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

// EmbeddedDataSpecificationsBuilder constructs EmbeddedDataSpecification objects from
// flattened database query results. It manages the incremental building of complex nested
// structures including references, IEC 61360 content, value lists, and level types.
//
// The builder maintains a map of data specifications indexed by their database IDs,
// allowing multiple database rows to contribute to the same specification. This is
// necessary because the normalized database structure splits embedded data specifications
// across multiple tables (references, content, value lists, etc.).
//
// Typical usage pattern:
//  1. Create builder with NewEmbeddedDataSpecificationsBuilder()
//  2. Call BuildReferences() to process reference data
//  3. Call BuildContentsIec61360() to process IEC 61360 content
//  4. Call Build() to extract the final slice of specifications
type EmbeddedDataSpecificationsBuilder struct {
	dataSpecifications map[int64]*embeddedDataSpecificationWithPosition
}

type embeddedDataSpecificationWithPosition struct {
	spec     gen.EmbeddedDataSpecification
	position int
}

// NewEmbeddedDataSpecificationsBuilder creates a new instance of EmbeddedDataSpecificationsBuilder
// with an initialized data specifications map ready to process database results.
//
// Returns:
//   - *EmbeddedDataSpecificationsBuilder: A new builder instance for constructing embedded
//     data specifications from database query results
//
// Example:
//
//	builder := NewEmbeddedDataSpecificationsBuilder()
//	err := builder.BuildReferences(refData, referredRefData)
//	if err != nil {
//	    // Handle error
//	}
//	err = builder.BuildContentsIec61360(iecData)
//	if err != nil {
//	    // Handle error
//	}
//	specs := builder.Build()
func NewEmbeddedDataSpecificationsBuilder() *EmbeddedDataSpecificationsBuilder {
	return &EmbeddedDataSpecificationsBuilder{
		dataSpecifications: make(map[int64]*embeddedDataSpecificationWithPosition),
	}
}

// BuildReferences processes reference data for embedded data specifications and constructs
// complete Reference objects with their hierarchical ReferredSemanticID structures.
//
// This method handles the DataSpecification field of EmbeddedDataSpecification objects,
// which points to the semantic definition of the data specification. It processes both
// direct references and referred references (nested references), building the complete
// reference hierarchy.
//
// Parameters:
//   - edsReferenceRows: JSON-encoded array of EdsReferenceRow objects containing reference
//     and key data from the database
//   - edsReferredReferenceRows: JSON-encoded array of ReferredReferenceRow objects containing
//     hierarchical referred reference data
//
// Returns:
//   - error: An error if unmarshalling fails, reference parsing fails, or if an embedded
//     data specification doesn't have exactly one reference. Returns nil on success.
//
// The method performs the following steps:
//  1. Unmarshals the reference row data
//  2. Creates placeholder EmbeddedDataSpecification entries for each unique EDS ID
//  3. Converts EdsReferenceRow objects to ReferenceRow format for processing
//  4. Parses references using ReferenceBuilder for each specification
//  5. Processes referred references to build hierarchical structures
//  6. Finalizes the nested reference structures
//
// Example:
//
//	builder := NewEmbeddedDataSpecificationsBuilder()
//	err := builder.BuildReferences(refJSON, referredRefJSON)
//	if err != nil {
//	    log.Printf("Failed to build references: %v", err)
//	}
func (edsb *EmbeddedDataSpecificationsBuilder) BuildReferences(edsReferenceRows json.RawMessage, edsReferredReferenceRows json.RawMessage) error {
	var edsRefRow []EdsReferenceRow
	if err := json.Unmarshal(edsReferenceRows, &edsRefRow); err != nil {
		return fmt.Errorf("failed to unmarshal edsReferenceRows: %w", err)
	}

	createEdsForEachDbEntryReferenceRow(edsRefRow, edsb)

	referenceBuilders := make(map[int64]*ReferenceBuilder)

	converted, err := createEdsIDReferenceMap(edsRefRow)
	if err != nil {
		return err
	}

	if err := edsb.parseEdsReferencesForEachEds(converted, referenceBuilders); err != nil {
		return err
	}

	if err := ParseReferredReferences(edsReferredReferenceRows, referenceBuilders); err != nil {
		return err
	}

	for _, refBuilder := range referenceBuilders {
		refBuilder.BuildNestedStructure()
	}

	return nil
}

// BuildContentsIec61360 processes IEC 61360 data specification content and populates the
// DataSpecificationContent field of each EmbeddedDataSpecification.
//
// This method handles the complex IEC 61360 data specification format, which includes:
//   - Multi-language preferred names, short names, and definitions
//   - Unit references with hierarchical structures
//   - Data types, value formats, and values
//   - Optional value lists with reference pairs
//   - Optional level types for hierarchical concepts
//
// Parameters:
//   - iecRows: JSON-encoded array of EdsContentIec61360Row objects containing IEC 61360
//     content data including language strings, references, value lists, and level types
//
// Returns:
//   - error: An error if unmarshalling fails, data type conversion fails, language string
//     parsing fails, reference building fails, or validation checks fail. Returns nil on success.
//
// The method performs comprehensive processing:
//  1. Unmarshals IEC 61360 content rows
//  2. Creates placeholder entries for each specification
//  3. For each IEC 61360 content:
//     - Converts data type from string to enum
//     - Parses multi-language strings (preferred name, short name, definition)
//     - Builds unit ID references with hierarchy
//     - Processes optional value lists with their references
//     - Parses optional level type information
//  4. Constructs DataSpecificationIec61360 objects
//  5. Attaches optional value lists and level types using setter methods
//
// Validation ensures:
//   - Exactly one unit ID reference per specification
//   - Exactly one reference per value list entry
//
// Example:
//
//	builder := NewEmbeddedDataSpecificationsBuilder()
//	builder.BuildReferences(refJSON, referredRefJSON)
//	err := builder.BuildContentsIec61360(iecJSON)
//	if err != nil {
//	    log.Printf("Failed to build IEC 61360 content: %v", err)
//	}
func (edsb *EmbeddedDataSpecificationsBuilder) BuildContentsIec61360(iecRows json.RawMessage) error {
	var iecContents []EdsContentIec61360Row
	if err := json.Unmarshal(iecRows, &iecContents); err != nil {
		return fmt.Errorf("failed to unmarshal iecRows: %w", err)
	}
	createEdsForEachDbEntryContent(iecContents, edsb)

	for _, data := range iecContents {
		edsWrapper := edsb.dataSpecifications[data.EdsID]

		preferredName, err := ParseLangStringPreferredNameTypeIec61360(data.PreferredName)
		if err != nil {
			return fmt.Errorf("error converting PreferredName for iec content %d", data.IecID)
		}

		if len(preferredName) == 0 {
			fmt.Print("Empty")
		}

		shortName, err := ParseLangStringShortNameTypeIec61360(data.ShortName)
		if err != nil {
			return fmt.Errorf("error converting ShortName for iec content %d", data.IecID)
		}

		definition, err := ParseLangStringDefinitionTypeIec61360(data.Definition)
		if err != nil {
			return fmt.Errorf("error converting Definition for iec content %d", data.IecID)
		}

		referenceBuilderMap, unitID, err := buildUnitID(data)
		if err != nil {
			return err
		}

		var valueList *gen.ValueList

		if valueList, err = edsb.addValueListIfSet(data, referenceBuilderMap); err != nil {
			return err
		}

		for _, refBuilder := range referenceBuilderMap {
			refBuilder.BuildNestedStructure()
		}

		var levelType *gen.LevelType
		if len(data.LevelType) == 0 {
			levelType = nil
		} else {
			if err := json.Unmarshal(data.LevelType, &levelType); err != nil {
				return fmt.Errorf("error converting LevelType for Embedded Data Specification Content ID %d: %w", data.IecID, err)
			}
		}

		edsWrapper.spec.DataSpecificationContent = &gen.DataSpecificationIec61360{
			ModelType:          "DataSpecificationIec61360",
			Unit:               data.Unit,
			SourceOfDefinition: data.SourceOfDefinition,
			Symbol:             data.Symbol,
			ValueFormat:        data.ValueFormat,
			Value:              data.Value,
			PreferredName:      preferredName,
			ShortName:          shortName,
			Definition:         definition,
		}

		if data.DataType != "" {
			dataType, err := gen.NewDataTypeIec61360FromValue(data.DataType)
			if err != nil {
				return fmt.Errorf("error converting DataType for iec content %d", data.IecID)
			}
			edsWrapper.spec.DataSpecificationContent.(*gen.DataSpecificationIec61360).DataType = dataType
		}

		if len(unitID) > 1 {
			return fmt.Errorf("expected exactly one or no UnitID reference for iec content %d, got %d", data.IecID, len(unitID))
		} else if len(unitID) == 1 {
			edsWrapper.spec.DataSpecificationContent.(*gen.DataSpecificationIec61360).UnitID = unitID[0]
		}

		// Store the position from the data
		edsWrapper.position = data.Position
		edsb.dataSpecifications[data.EdsID] = edsWrapper

		if valueList != nil {
			edsb.dataSpecifications[data.EdsID].spec.DataSpecificationContent.SetValueList(valueList)
		}
		if levelType != nil {
			edsb.dataSpecifications[data.EdsID].spec.DataSpecificationContent.SetLevelType(levelType)
		}
	}

	return nil
}

func buildUnitID(data EdsContentIec61360Row) (map[int64]*ReferenceBuilder, []*gen.Reference, error) {
	referenceBuilderMap := make(map[int64]*ReferenceBuilder)

	unitID, err := ParseReferences(data.UnitReferenceKeys, referenceBuilderMap)
	if err != nil {
		return nil, nil, fmt.Errorf("error converting UnitID reference for iec content %d: %w", data.IecID, err)
	}
	err = ParseReferredReferences(data.UnitReferenceReferred, referenceBuilderMap)
	if err != nil {
		return nil, nil, fmt.Errorf("error converting referred UnitID reference for iec content %d: %w", data.IecID, err)
	}
	return referenceBuilderMap, unitID, nil
}

func (*EmbeddedDataSpecificationsBuilder) addValueListIfSet(data EdsContentIec61360Row, referenceBuilderMap map[int64]*ReferenceBuilder) (*gen.ValueList, error) {
	if len(data.ValueListEntries) > 0 {
		var valueListRows []ValueListRow
		if err := json.Unmarshal(data.ValueListEntries, &valueListRows); err != nil {
			return nil, fmt.Errorf("failed to unmarshal ValueListEntries for iec content %d: %w", data.IecID, err)
		}
		valueList := &gen.ValueList{
			ValueReferencePairs: []*gen.ValueReferencePair{},
		}
		for _, entry := range valueListRows {
			reference, err := ParseReferences(entry.ReferenceRows, referenceBuilderMap)
			if err != nil {
				return nil, fmt.Errorf("error parsing Reference for ValueReferencePair with ID %d", entry.ValueRefPairID)
			}
			err = ParseReferredReferences(entry.ReferredReferenceRows, referenceBuilderMap)
			if err != nil {
				return nil, fmt.Errorf("error parsing ReferredReference for ValueReferencePair with ID %d: %w", entry.ValueRefPairID, err)
			}
			if len(reference) != 1 {
				return nil, fmt.Errorf("expected exactly one reference for ValueReferencePair ID %d, got %d", entry.ValueRefPairID, len(reference))
			}
			pair := gen.ValueReferencePair{
				Value:   entry.Value,
				ValueID: reference[0],
			}
			valueList.ValueReferencePairs = append(valueList.ValueReferencePairs, &pair)
		}
		// Check if at least one entry was added
		if len(valueList.ValueReferencePairs) == 0 {
			return nil, nil
		}
		return valueList, nil
	}
	return nil, nil
}

// Build finalizes the construction of all embedded data specifications and returns them as a slice.
// This method should be called after all data specifications and their contents have been processed
// through BuildReferences() and BuildContentsIec61360().
//
// The method extracts all embedded data specifications from the internal map and returns them
// as a slice. Each specification contains complete reference hierarchies and IEC 61360 content
// where applicable.
//
// Returns:
//   - []gen.EmbeddedDataSpecification: A slice containing all constructed embedded data specifications
//     with their complete reference hierarchies and content
//
// Example:
//
//	builder := NewEmbeddedDataSpecificationsBuilder()
//	builder.BuildReferences(refData, referredRefData)
//	builder.BuildContentsIec61360(iecData)
//	specs := builder.Build()
func (edsb *EmbeddedDataSpecificationsBuilder) Build() []gen.EmbeddedDataSpecification {
	specList := make([]*embeddedDataSpecificationWithPosition, 0, len(edsb.dataSpecifications))
	for _, specWrapper := range edsb.dataSpecifications {
		specList = append(specList, specWrapper)
	}

	// Sort by position
	sort.Slice(specList, func(i, j int) bool {
		return specList[i].position < specList[j].position
	})

	result := make([]gen.EmbeddedDataSpecification, 0, len(specList))
	for _, specWrapper := range specList {
		result = append(result, specWrapper.spec)
	}
	return result
}

func createEdsForEachDbEntryContent(edsRefRow []EdsContentIec61360Row, edsb *EmbeddedDataSpecificationsBuilder) {
	for _, edsRef := range edsRefRow {
		if _, exists := edsb.dataSpecifications[edsRef.EdsID]; !exists {
			edsb.dataSpecifications[edsRef.EdsID] = &embeddedDataSpecificationWithPosition{
				spec:     gen.EmbeddedDataSpecification{},
				position: 0, // Will be set in BuildContentsIec61360
			}
		}
	}
}

func createEdsForEachDbEntryReferenceRow(edsRefRow []EdsReferenceRow, edsb *EmbeddedDataSpecificationsBuilder) {
	for _, edsRef := range edsRefRow {
		if _, exists := edsb.dataSpecifications[edsRef.EdsID]; !exists {
			edsb.dataSpecifications[edsRef.EdsID] = &embeddedDataSpecificationWithPosition{
				spec:     gen.EmbeddedDataSpecification{},
				position: 0, // Will be set when content is added
			}
		}
	}
}

func createEdsIDReferenceMap(edsRefRows []EdsReferenceRow) (map[int64][]ReferenceRow, error) {
	converted := make(map[int64][]ReferenceRow)
	for _, ref := range edsRefRows {
		if ref.ReferenceType == nil {
			return nil, fmt.Errorf("reference type is nil for edsID %d", ref.EdsID)
		}
		refRow := ReferenceRow{
			ReferenceID:   ref.ReferenceID,
			ReferenceType: *ref.ReferenceType,
			KeyID:         ref.KeyID,
			KeyType:       ref.KeyType,
			KeyValue:      ref.KeyValue,
		}
		converted[ref.EdsID] = append(converted[ref.EdsID], refRow)
	}
	return converted, nil
}

func (edsb *EmbeddedDataSpecificationsBuilder) parseEdsReferencesForEachEds(edsIDReferenceRowMapping map[int64][]ReferenceRow, referenceBuilders map[int64]*ReferenceBuilder) error {
	for edsID, refs := range edsIDReferenceRowMapping {
		refsParsed := ParseReferencesFromRows(refs, referenceBuilders)
		if len(refsParsed) == 1 {
			edsSpecWrapper := edsb.dataSpecifications[edsID]
			edsSpecWrapper.spec.DataSpecification = refsParsed[0]
			edsb.dataSpecifications[edsID] = edsSpecWrapper
		} else {
			return fmt.Errorf("expected exactly one reference for edsID %d, got %d", edsID, len(refsParsed))
		}
	}
	return nil
}
