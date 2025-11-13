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

// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

// Package builder provides utilities for converting SQL query results into Go data structures.
// It contains types and functions to handle the transformation of database rows into
// BaSyx-compliant data models, including handling of complex nested structures like
// references, language strings, and embedded data specifications.
package builder

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
)

// AssetAdministrationShellDescriptorRow represents a single SQL result row
// for an Asset Administration Shell (AAS) descriptor. It carries nullable
// string/integer columns from the database and foreign-key references to
// related records such as administrative information, display names, and
// descriptions.
type AssetAdministrationShellDescriptorRow struct {
	DescID        int64
	AssetKindStr  sql.NullString
	AssetType     sql.NullString
	GlobalAssetID sql.NullString
	IDShort       sql.NullString
	IDStr         string
	AdminInfoID   sql.NullInt64
	DisplayNameID sql.NullInt64
	DescriptionID sql.NullInt64
}

// SubmodelDescriptorRow represents a single SQL result row for a Submodel
// descriptor that is associated with an AAS descriptor. It includes the
// database identifiers of the AAS and Submodel descriptors as well as
// optional columns and foreign-key references such as semantic reference
// and administrative information.
type SubmodelDescriptorRow struct {
	AasDescID     int64
	SmdDescID     int64
	IDShort       sql.NullString
	ID            sql.NullString
	SemanticRefID sql.NullInt64
	AdminInfoID   sql.NullInt64
	DescriptionID sql.NullInt64
	DisplayNameID sql.NullInt64
}
// ParseReferredReferencesFromRows parses referred reference data from already unmarshalled ReferredReferenceRow objects.
//
// This function handles the complex case where references point to other references (referred references).
// It validates that parent references exist in the builder map before creating child references,
// ensuring referential integrity in the hierarchical structure.
//
// Parameters:
//   - semanticIdData: Slice of already unmarshalled ReferredReferenceRow objects
//   - referenceBuilderRefs: Map of reference IDs to their corresponding ReferenceBuilder instances.
//     This map is used to look up parent references and must be pre-populated with root references.
//   - mu: Optional mutex for concurrent access protection. If nil, no locking is performed.
//
// Returns:
//   - error: An error if a parent reference is not found in the map.
//     Nil references or keys are logged as warnings but do not cause the function to fail.
//
// The function performs the following validations:
//   - Skips entries with nil RootReference, ReferenceID, ParentReference, or ReferenceType
//   - Verifies parent references exist in the builder map
//   - Ensures key data (KeyID, KeyType, KeyValue) is complete
func ParseReferredReferencesFromRows(semanticIDData []model.ReferredReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder, mu *sync.RWMutex) error {
	for _, ref := range semanticIDData {
		if ref.RootReference == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] RootReference was nil - skipping Reference Creation.")
			continue
		}

		// Read lock to check if builder exists
		if mu != nil {
			mu.RLock()
		}
		builder, semanticIDCreated := referenceBuilderRefs[*ref.RootReference]
		if mu != nil {
			mu.RUnlock()
		}

		if !semanticIDCreated {
			return fmt.Errorf("parent reference with id %d not found for referred reference with id %d", ref.ParentReference, ref.ReferenceID)
		}
		if ref.ReferenceID == nil || ref.ParentReference == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] ReferenceID or ParentReference was nil - skipping Reference Creation.")
			continue
		}
		if ref.ReferenceType == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] ReferenceType was nil - skipping Reference Creation for Reference with Reference ID", *ref.ReferenceID)
			continue
		}
		if ref.KeyID == nil || ref.KeyType == nil || ref.KeyValue == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] KeyID, KeyType or KeyValue was nil - skipping Reference Creation for Reference with Reference ID", *ref.ReferenceID)
			continue
		}
		builder.CreateReferredSemanticID(*ref.ReferenceID, *ref.ParentReference, *ref.ReferenceType)
		err := builder.CreateReferredSemanticIDKey(*ref.ReferenceID, *ref.KeyID, *ref.KeyType, *ref.KeyValue)

		if err != nil {
			return fmt.Errorf("error creating key for referred reference with id %d: %w", *ref.ReferenceID, err)
		}
	}
	return nil
}

// ParseReferredReferences parses referred reference data from JSON and populates the reference builder map.
//
// This function unmarshals JSON-encoded ReferredReferenceRow data and delegates to ParseReferredReferencesFromRows
// for the actual parsing logic.
//
// Parameters:
//   - row: JSON-encoded array of ReferredReferenceRow objects from the database
//   - referenceBuilderRefs: Map of reference IDs to their corresponding ReferenceBuilder instances.
//     This map is used to look up parent references and must be pre-populated with root references.
//   - mu: Optional mutex for concurrent access protection. If nil, no locking is performed.
//
// Returns:
//   - error: An error if JSON unmarshalling fails or if a parent reference is not found in the map.
//     Nil references or keys are logged as warnings but do not cause the function to fail.
func ParseReferredReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder, mu *sync.RWMutex) error {
	if len(row) == 0 {
		return nil
	}

	var semanticIDData []model.ReferredReferenceRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &semanticIDData); err != nil {
		return fmt.Errorf("error unmarshalling referred semantic ID data: %w", err)
	}

	return ParseReferredReferencesFromRows(semanticIDData, referenceBuilderRefs, mu)
}

// ParseReferencesFromRows parses reference data from already unmarshalled ReferenceRow objects.
//
// This function processes an array of ReferenceRow objects and builds complete Reference
// objects with their associated Keys. Multiple rows with the same ReferenceID are aggregated
// into a single Reference object with multiple Keys.
//
// Parameters:
//   - semanticIdData: Slice of already unmarshalled ReferenceRow objects
//   - referenceBuilderRefs: Map that tracks reference IDs to their corresponding ReferenceBuilder instances.
//     This map is populated by this function and can be used later for processing referred references.
//   - mu: Optional mutex for concurrent access protection. If nil, no locking is performed.
//
// Returns:
//   - []*model.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//
// The function:
//   - Groups multiple rows with the same ReferenceID into a single Reference
//   - Creates new ReferenceBuilder instances for each unique ReferenceId
//   - Validates key data completeness (KeyID, KeyType, KeyValue)
//   - Returns only the unique references (one per ReferenceID)
func ParseReferencesFromRows(semanticIDData []model.ReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder, mu *sync.RWMutex) []*model.Reference {
	resultArray := make([]*model.Reference, 0)

	for _, ref := range semanticIDData {
		var semanticID *model.Reference
		var semanticIDBuilder *ReferenceBuilder

		// Check if reference already exists
		if mu != nil {
			mu.RLock()
		}
		_, semanticIDCreated := referenceBuilderRefs[ref.ReferenceID]
		if mu != nil {
			mu.RUnlock()
		}

		if !semanticIDCreated {
			semanticID, semanticIDBuilder = NewReferenceBuilder(ref.ReferenceType, ref.ReferenceID)

			// Write lock to add to map
			if mu != nil {
				mu.Lock()
			}
			referenceBuilderRefs[ref.ReferenceID] = semanticIDBuilder
			if mu != nil {
				mu.Unlock()
			}

			resultArray = append(resultArray, semanticID)
		} else {
			// Read lock to get from map
			if mu != nil {
				mu.RLock()
			}
			semanticIDBuilder = referenceBuilderRefs[ref.ReferenceID]
			if mu != nil {
				mu.RUnlock()
			}
		}

		if ref.KeyID == nil || ref.KeyType == nil || ref.KeyValue == nil {
			fmt.Println("[WARNING - ParseReferencesFromRows] KeyID, KeyType or KeyValue was nil - skipping Key Creation for Reference with Reference ID", ref.ReferenceID)
			continue
		}
		semanticIDBuilder.CreateKey(*ref.KeyID, *ref.KeyType, *ref.KeyValue)
	}

	return resultArray
}

// ParseReferences parses reference data from JSON and creates Reference objects.
//
// This function unmarshals JSON-encoded ReferenceRow data and delegates to ParseReferencesFromRows
// for the actual parsing logic. Multiple rows with the same ReferenceID are aggregated into a
// single Reference object with multiple Keys.
//
// Parameters:
//   - row: JSON-encoded array of ReferenceRow objects from the database
//   - referenceBuilderRefs: Map that tracks reference IDs to their corresponding ReferenceBuilder instances.
//     This map is populated by this function and can be used later for processing referred references.
//   - mu: Optional mutex for concurrent access protection. If nil, no locking is performed.
//
// Returns:
//   - []*model.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//   - error: An error if JSON unmarshalling fails. Nil key data is logged as warnings but does not cause failure.
func ParseReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder, mu *sync.RWMutex) ([]*model.Reference, error) {
	if len(row) == 0 {
		return make([]*model.Reference, 0), nil
	}

	var semanticIDData []model.ReferenceRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &semanticIDData); err != nil {
		return nil, fmt.Errorf("error unmarshalling semantic ID data: %w", err)
	}

	return ParseReferencesFromRows(semanticIDData, referenceBuilderRefs, mu), nil
}

// ParseLangStringNameType parses localized name strings from JSON data.
//
// This function converts JSON-encoded language-specific name data from the database
// into a slice of LangStringNameType objects. It removes internal database IDs from
// the data before creating the Go structures.
//
// Parameters:
//   - displayNames: JSON-encoded array of objects containing id, text, and language fields
//
// Returns:
//   - []model.LangStringNameType: Slice of parsed language-specific name objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function:
//   - Unmarshals JSON into temporary map structures
//   - Removes the internal 'id' field used for database relationships
//   - Creates LangStringNameType objects with text and language fields
//   - Uses panic recovery to handle runtime errors during type assertions
//
// Note: Only objects with an 'id' field are processed to ensure data integrity.
func ParseLangStringNameType(displayNames json.RawMessage) ([]model.LangStringNameType, error) {
	var names []model.LangStringNameType
	// remove id field from json
	var temp []map[string]interface{}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(displayNames, &temp); err != nil {
		fmt.Printf("Error unmarshalling display names: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing display names: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			names = append(names, model.LangStringNameType{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return names, nil
}

// ParseLangStringTextType parses localized text strings from JSON data.
//
// This function converts JSON-encoded language-specific text data (such as descriptions)
// from the database into a slice of LangStringTextType objects. It removes internal
// database IDs from the data before creating the Go structures.
//
// Parameters:
//   - descriptions: JSON-encoded array of objects containing id, text, and language fields
//
// Returns:
//   - []model.LangStringTextType: Slice of parsed language-specific text objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function:
//   - Unmarshals JSON into temporary map structures
//   - Removes the internal 'id' field used for database relationships
//   - Creates LangStringTextType objects with text and language fields
//   - Uses panic recovery to handle runtime errors during type assertions
//
// Note: Only objects with an 'id' field are processed to ensure data integrity.
// This function is similar to ParseLangStringNameType but produces LangStringTextType
// objects which may have different validation rules or usage contexts.
func ParseLangStringTextType(descriptions json.RawMessage) ([]model.LangStringTextType, error) {
	var texts []model.LangStringTextType
	// remove id field from json
	var temp []map[string]interface{}
	if len(descriptions) == 0 {
		return texts, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(descriptions, &temp); err != nil {
		fmt.Printf("Error unmarshalling descriptions: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing descriptions: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			texts = append(texts, model.LangStringTextType{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

// ParseLangStringPreferredNameTypeIec61360 parses localized preferred names for IEC 61360 data specifications from JSON data.
//
// This function converts JSON-encoded language-specific preferred name data from the database
// into a slice of LangStringPreferredNameTypeIec61360 objects. It removes internal database IDs
// from the data before creating the Go structures.
//
// Parameters:
//   - descriptions: JSON-encoded array of objects containing id, text, and language fields
//
// Returns:
//   - []model.LangStringPreferredNameTypeIec61360: Slice of parsed language-specific preferred name objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function handles empty input by returning an empty slice. It uses panic recovery to
// handle runtime errors during type assertions. Only objects with an 'id' field are processed
// to ensure data integrity.
func ParseLangStringPreferredNameTypeIec61360(descriptions json.RawMessage) ([]model.LangStringPreferredNameTypeIec61360, error) {
	var texts []model.LangStringPreferredNameTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
	if len(descriptions) == 0 {
		return texts, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(descriptions, &temp); err != nil {
		fmt.Printf("Error unmarshalling descriptions: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing descriptions: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			texts = append(texts, model.LangStringPreferredNameTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

// ParseLangStringShortNameTypeIec61360 parses localized short names for IEC 61360 data specifications from JSON data.
//
// This function converts JSON-encoded language-specific short name data from the database
// into a slice of LangStringShortNameTypeIec61360 objects. It removes internal database IDs
// from the data before creating the Go structures.
//
// Parameters:
//   - descriptions: JSON-encoded array of objects containing id, text, and language fields
//
// Returns:
//   - []model.LangStringShortNameTypeIec61360: Slice of parsed language-specific short name objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function handles empty input by returning an empty slice. It uses panic recovery to
// handle runtime errors during type assertions. Only objects with an 'id' field are processed
// to ensure data integrity.
func ParseLangStringShortNameTypeIec61360(descriptions json.RawMessage) ([]model.LangStringShortNameTypeIec61360, error) {
	var texts []model.LangStringShortNameTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
	if len(descriptions) == 0 {
		return texts, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(descriptions, &temp); err != nil {
		fmt.Printf("Error unmarshalling descriptions: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing descriptions: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			texts = append(texts, model.LangStringShortNameTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

// ParseLangStringDefinitionTypeIec61360 parses localized definitions for IEC 61360 data specifications from JSON data.
//
// This function converts JSON-encoded language-specific definition data from the database
// into a slice of LangStringDefinitionTypeIec61360 objects. It removes internal database IDs
// from the data before creating the Go structures.
//
// Parameters:
//   - descriptions: JSON-encoded array of objects containing id, text, and language fields
//
// Returns:
//   - []model.LangStringDefinitionTypeIec61360: Slice of parsed language-specific definition objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function handles empty input by returning an empty slice. It uses panic recovery to
// handle runtime errors during type assertions. Only objects with an 'id' field are processed
// to ensure data integrity.
func ParseLangStringDefinitionTypeIec61360(descriptions json.RawMessage) ([]model.LangStringDefinitionTypeIec61360, error) {
	var texts []model.LangStringDefinitionTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
	if len(descriptions) == 0 {
		return texts, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(descriptions, &temp); err != nil {
		fmt.Printf("Error unmarshalling descriptions: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing descriptions: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			texts = append(texts, model.LangStringDefinitionTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

// ParseQualifiersRow parses qualifier data from JSON into QualifierRow objects.
//
// This function unmarshals JSON-encoded qualifier data from the database into a slice
// of QualifierRow objects. Each row represents a single qualifier with its associated
// semantic IDs, value IDs, and supplemental semantic IDs stored as nested JSON.
//
// Parameters:
//   - row: JSON-encoded array of QualifierRow objects from the database
//
// Returns:
//   - []QualifierRow: Slice of parsed QualifierRow objects
//   - error: An error if JSON unmarshalling fails
func ParseQualifiersRow(row json.RawMessage) ([]model.QualifierRow, error) {
	var texts []model.QualifierRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling qualifier data: %w", err)
	}
	return texts, nil
}

// ParseExtensionRows parses extension data from JSON into ExtensionRow objects.
//
// This function unmarshals JSON-encoded extension data from the database into a slice
// of ExtensionRow objects. Each row represents a single extension with its associated
// semantic IDs, supplemental semantic IDs, and references stored as nested JSON.
//
// Parameters:
//   - row: JSON-encoded array of ExtensionRow objects from the database
//
// Returns:
//   - []ExtensionRow: Slice of parsed ExtensionRow objects
//   - error: An error if JSON unmarshalling fails
func ParseExtensionRows(row json.RawMessage) ([]model.ExtensionRow, error) {
	var texts []model.ExtensionRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling extension data: %w", err)
	}
	return texts, nil
}

// ParseAdministrationRow parses administrative information from JSON into an AdministrationRow object.
//
// This function unmarshals JSON-encoded administrative data from the database. Since
// administrative information is typically singular for an element, it returns a pointer
// to a single AdministrationRow object or nil if no data is present.
//
// Parameters:
//   - row: JSON-encoded array of AdministrationRow objects from the database
//
// Returns:
//   - *AdministrationRow: Pointer to the parsed AdministrationRow object, or nil if no data
//   - error: An error if JSON unmarshalling fails
//
// Note: The function expects an array in JSON format but returns only the first element,
// as administrative information is singular per element.
func ParseAdministrationRow(row json.RawMessage) (*model.AdministrationRow, error) {
	var texts []model.AdministrationRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling AdministrationRow data: %w", err)
	}
	if len(texts) == 0 {
		return nil, nil
	}
	return &texts[0], nil
}
