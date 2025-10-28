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
	"encoding/json"
	"fmt"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// SubmodelRow represents a row from the Submodel table in the database.
// JSON fields are represented as json.RawMessage to allow for deferred parsing
// and handling of complex nested structures.
//
// This structure is used when retrieving submodel data from SQL queries,
// where complex fields like DisplayNames, Descriptions, and References are
// stored as JSON in the database and need to be parsed separately.
type SubmodelRow struct {
	// Id is the unique identifier of the submodel
	Id string
	// IdShort is the short identifier for the submodel
	IdShort string
	// Category defines the category classification of the submodel
	Category string
	// Kind specifies whether the submodel is a Template or Instance
	Kind string
	// DisplayNames contains localized names as JSON data
	DisplayNames json.RawMessage
	// Descriptions contains localized descriptions as JSON data
	Descriptions json.RawMessage
	// SemanticId is a reference to a semantic definition as JSON data
	SemanticId json.RawMessage
	// ReferredSemanticIds contains references to additional semantic definitions as JSON data
	ReferredSemanticIds json.RawMessage
	// SupplementalSemanticIds contains supplemental semantic identifiers as JSON data
	SupplementalSemanticIds json.RawMessage
	// SupplementalReferredSemIds contains referred supplemental semantic identifiers as JSON data
	SupplementalReferredSemIds json.RawMessage
	// DataSpecReference contains embedded data specifications as JSON data
	DataSpecReference json.RawMessage
	// DataSpecReferenceReferred contains references to data specifications as JSON data
	DataSpecReferenceReferred json.RawMessage
	// DataSpecIEC61360 contains IEC 61360 data specification as JSON data
	DataSpecIEC61360 json.RawMessage
	// IECLevelTypes contains IEC level type information as JSON data
	IECLevelTypes json.RawMessage
	// Qualifiers contains qualifier information as JSON data
	Qualifiers json.RawMessage
	// Extensions contains extension as JSON data
	Extensions json.RawMessage
	// Administration contains administrative information as JSON data
	Administration json.RawMessage
	// RootSubmodelElements contains root submodel elements as JSON data
	RootSubmodelElements json.RawMessage
	// ChildSubmodelElements contains child submodel elements as JSON data
	ChildSubmodelElements json.RawMessage
	// TotalSubmodels is the total count of submodels in the result set
	TotalSubmodels int64
}

// ReferenceRow represents a data row for a Reference entity in the database.
// There will be multiple ReferenceRow entries for each Reference, one for each Key
// associated with that Reference.
//
// In the AAS metamodel, a Reference consists of multiple Keys that form a path.
// The database stores these as separate rows, which are then aggregated during parsing.
//
// Example: If you have 1 Reference with 3 Keys, there will be 3 ReferenceRow entries
// with the same ReferenceId and ReferenceType but different Key details.
type ReferenceRow struct {
	// ReferenceId is the unique identifier of the reference in the database
	ReferenceId int64 `json:"reference_id"`
	// ReferenceType specifies the type of reference (e.g., ExternalReference, ModelReference)
	ReferenceType string `json:"reference_type"`
	// KeyID is the unique identifier of the key in the database (nullable)
	KeyID *int64 `json:"key_id"`
	// KeyType specifies the type of the key (e.g., Submodel, Property) (nullable)
	KeyType *string `json:"key_type"`
	// KeyValue contains the actual value of the key (nullable)
	KeyValue *string `json:"key_value"`
}

type EdsReferenceRow struct {
	// EdsID is the unique identifier of the embedded data specification in the database
	EdsID int64 `json:"eds_id"`
	// ReferenceId is the unique identifier of the reference in the database
	ReferenceId int64 `json:"reference_id"`
	// ReferenceType specifies the type of reference (nullable)
	ReferenceType *string `json:"reference_type"`
	// KeyID is the unique identifier of the key in the database (nullable)
	KeyID *int64 `json:"key_id"`
	// KeyType specifies the type of the key (nullable)
	KeyType *string `json:"key_type"`
	// KeyValue contains the actual value of the key (nullable)
	KeyValue *string `json:"key_value"`
}

// ReferredReferenceRow represents a data row for a referred Reference entity in the database.
// There will be multiple ReferredReferenceRow entries for each referred Reference, one for
// each Key associated with that referred Reference.
//
// Referred references are used in contexts where references point to other references,
// creating a hierarchical structure. This is common in supplemental semantic IDs where
// multiple references can be associated with a semantic concept.
//
// Example: If you have 1 referred Reference with 2 Keys, there will be 2 ReferredReferenceRow
// entries with the same ReferenceId and ReferenceType but different Key details.
type ReferredReferenceRow struct {
	// SupplementalRootReferenceId identifies the root supplemental reference (nullable)
	SupplementalRootReferenceId *int64 `json:"supplemental_root_reference_id"`
	// ReferenceId is the unique identifier of this reference in the database (nullable)
	ReferenceId *int64 `json:"reference_id"`
	// ReferenceType specifies the type of reference (nullable)
	ReferenceType *string `json:"reference_type"`
	// ParentReference identifies the parent reference in the hierarchy (nullable)
	ParentReference *int64 `json:"parentReference"`
	// RootReference identifies the root reference in the hierarchy (nullable)
	RootReference *int64 `json:"rootReference"`
	// KeyID is the unique identifier of the key in the database (nullable)
	KeyID *int64 `json:"key_id"`
	// KeyType specifies the type of the key (nullable)
	KeyType *string `json:"key_type"`
	// KeyValue contains the actual value of the key (nullable)
	KeyValue *string `json:"key_value"`
}

type EdsContentIec61360Row struct {
	EdsID                 int64           `json:"eds_id"`
	IecID                 int64           `json:"iec_id"`
	Unit                  string          `json:"unit"`
	SourceOfDefinition    string          `json:"source_of_definition"`
	Symbol                string          `json:"symbol"`
	DataType              string          `json:"data_type"`
	ValueFormat           string          `json:"value_format"`
	Value                 string          `json:"value"`
	LevelType             json.RawMessage `json:"level_type"`
	PreferredName         json.RawMessage `json:"preferred_name"`
	ShortName             json.RawMessage `json:"short_name"`
	Definition            json.RawMessage `json:"definition"`
	UnitReferenceKeys     json.RawMessage `json:"unit_reference_keys"`
	UnitReferenceReferred json.RawMessage `json:"unit_reference_referred"`
	ValueListEntries      json.RawMessage `json:"value_list_entries"`
}

type ValueListRow struct {
	Value                 string          `json:"value_pair_value"`
	ValueRefPairId        int64           `json:"value_reference_pair_id"`
	ReferenceRows         json.RawMessage `json:"reference_rows"`
	ReferredReferenceRows json.RawMessage `json:"referred_reference_rows"`
}

type QualifierRow struct {
	DbId                                      int64           `json:"dbId"`
	Kind                                      string          `json:"kind"`
	Type                                      string          `json:"type"`
	ValueType                                 string          `json:"value_type"`
	Value                                     string          `json:"value"`
	SemanticId                                json.RawMessage `json:"semanticIdReferenceRows"`
	SemanticIdReferredReferences              json.RawMessage `json:"semanticIdReferredReferencesRows"`
	ValueId                                   json.RawMessage `json:"valueIdReferenceRows"`
	ValueIdReferredReferences                 json.RawMessage `json:"valueIdReferredReferencesRows"`
	SupplementalSemanticIds                   json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	SupplementalSemanticIdsReferredReferences json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
}

type ExtensionRow struct {
	DbId                                      int64           `json:"dbId"`
	Name                                      string          `json:"name"`
	ValueType                                 string          `json:"value_type"`
	Value                                     string          `json:"value"`
	SemanticId                                json.RawMessage `json:"semanticIdReferenceRows"`
	SemanticIdReferredReferences              json.RawMessage `json:"semanticIdReferredReferencesRows"`
	SupplementalSemanticIds                   json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	SupplementalSemanticIdsReferredReferences json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
	RefersTo                                  json.RawMessage `json:"refersToReferenceRows"`
	RefersToReferredReferences                json.RawMessage `json:"refersToReferredReferencesRows"`
}

type AdministrationRow struct {
	DbId                          int64           `json:"dbId"`
	Version                       string          `json:"version"`
	Revision                      string          `json:"revision"`
	TemplateId                    string          `json:"templateId"`
	Creator                       json.RawMessage `json:"creator"`
	CreatorReferred               json.RawMessage `json:"creatorReferred"`
	EdsDataSpecifications         json.RawMessage `json:"edsDataSpecifications"`
	EdsDataSpecificationsReferred json.RawMessage `json:"edsDataSpecificationsReferred"`
	EdsDataSpecificationIEC61360  json.RawMessage `json:"edsDataSpecificationIEC61360"` //iecRows
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
//
// Returns:
//   - error: An error if a parent reference is not found in the map.
//     Nil references or keys are logged as warnings but do not cause the function to fail.
//
// The function performs the following validations:
//   - Skips entries with nil RootReference, ReferenceId, ParentReference, or ReferenceType
//   - Verifies parent references exist in the builder map
//   - Ensures key data (KeyID, KeyType, KeyValue) is complete
func ParseReferredReferencesFromRows(semanticIdData []ReferredReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder) error {
	for _, ref := range semanticIdData {
		if ref.RootReference == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] RootReference was nil - skipping Reference Creation.")
			continue
		}
		builder, semanticIdCreated := referenceBuilderRefs[*ref.RootReference]
		if !semanticIdCreated {
			return fmt.Errorf("parent reference with id %d not found for referred reference with id %d", ref.ParentReference, ref.ReferenceId)
		}
		if ref.ReferenceId == nil || ref.ParentReference == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] ReferenceId or ParentReference was nil - skipping Reference Creation.")
			continue
		}
		if ref.ReferenceType == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] ReferenceType was nil - skipping Reference Creation for Reference with Reference ID", *ref.ReferenceId)
			continue
		}
		if ref.KeyID == nil || ref.KeyType == nil || ref.KeyValue == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] KeyID, KeyType or KeyValue was nil - skipping Reference Creation for Reference with Reference ID", *ref.ReferenceId)
			continue
		}
		builder.CreateReferredSemanticId(*ref.ReferenceId, *ref.ParentReference, *ref.ReferenceType)
		builder.CreateReferredSemanticIdKey(*ref.ReferenceId, *ref.KeyID, *ref.KeyType, *ref.KeyValue)
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
//
// Returns:
//   - error: An error if JSON unmarshalling fails or if a parent reference is not found in the map.
//     Nil references or keys are logged as warnings but do not cause the function to fail.
func ParseReferredReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder) error {
	if len(row) == 0 {
		return nil
	}

	var semanticIdData []ReferredReferenceRow
	if err := json.Unmarshal(row, &semanticIdData); err != nil {
		return fmt.Errorf("error unmarshalling referred semantic ID data: %w", err)
	}

	return ParseReferredReferencesFromRows(semanticIdData, referenceBuilderRefs)
}

// ParseReferencesFromRows parses reference data from already unmarshalled ReferenceRow objects.
//
// This function processes an array of ReferenceRow objects and builds complete Reference
// objects with their associated Keys. Multiple rows with the same ReferenceId are aggregated
// into a single Reference object with multiple Keys.
//
// Parameters:
//   - semanticIdData: Slice of already unmarshalled ReferenceRow objects
//   - referenceBuilderRefs: Map that tracks reference IDs to their corresponding ReferenceBuilder instances.
//     This map is populated by this function and can be used later for processing referred references.
//
// Returns:
//   - []*gen.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//
// The function:
//   - Groups multiple rows with the same ReferenceId into a single Reference
//   - Creates new ReferenceBuilder instances for each unique ReferenceId
//   - Validates key data completeness (KeyID, KeyType, KeyValue)
//   - Returns only the unique references (one per ReferenceId)
func ParseReferencesFromRows(semanticIdData []ReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder) []*gen.Reference {
	resultArray := make([]*gen.Reference, 0)

	for _, ref := range semanticIdData {
		var semanticId *gen.Reference
		var semanticIdBuilder *ReferenceBuilder

		_, semanticIdCreated := referenceBuilderRefs[ref.ReferenceId]

		if !semanticIdCreated {
			semanticId, semanticIdBuilder = NewReferenceBuilder(ref.ReferenceType, ref.ReferenceId)
			referenceBuilderRefs[ref.ReferenceId] = semanticIdBuilder
			resultArray = append(resultArray, semanticId)
		} else {
			semanticIdBuilder = referenceBuilderRefs[ref.ReferenceId]
		}

		if ref.KeyID == nil || ref.KeyType == nil || ref.KeyValue == nil {
			fmt.Println("[WARNING - ParseReferencesFromRows] KeyID, KeyType or KeyValue was nil - skipping Key Creation for Reference with Reference ID", ref.ReferenceId)
			continue
		}
		semanticIdBuilder.CreateKey(*ref.KeyID, *ref.KeyType, *ref.KeyValue)
	}

	return resultArray
}

// ParseReferences parses reference data from JSON and creates Reference objects.
//
// This function unmarshals JSON-encoded ReferenceRow data and delegates to ParseReferencesFromRows
// for the actual parsing logic. Multiple rows with the same ReferenceId are aggregated into a
// single Reference object with multiple Keys.
//
// Parameters:
//   - row: JSON-encoded array of ReferenceRow objects from the database
//   - referenceBuilderRefs: Map that tracks reference IDs to their corresponding ReferenceBuilder instances.
//     This map is populated by this function and can be used later for processing referred references.
//
// Returns:
//   - []*gen.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//   - error: An error if JSON unmarshalling fails. Nil key data is logged as warnings but does not cause failure.
func ParseReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder) ([]*gen.Reference, error) {
	if len(row) == 0 {
		return make([]*gen.Reference, 0), nil
	}

	var semanticIdData []ReferenceRow
	if err := json.Unmarshal(row, &semanticIdData); err != nil {
		return nil, fmt.Errorf("error unmarshalling semantic ID data: %w", err)
	}

	return ParseReferencesFromRows(semanticIdData, referenceBuilderRefs), nil
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
//   - []gen.LangStringNameType: Slice of parsed language-specific name objects
//   - error: An error if JSON unmarshalling fails or if required fields are missing
//
// The function:
//   - Unmarshals JSON into temporary map structures
//   - Removes the internal 'id' field used for database relationships
//   - Creates LangStringNameType objects with text and language fields
//   - Uses panic recovery to handle runtime errors during type assertions
//
// Note: Only objects with an 'id' field are processed to ensure data integrity.
func ParseLangStringNameType(displayNames json.RawMessage) ([]gen.LangStringNameType, error) {
	var names []gen.LangStringNameType
	// remove id field from json
	var temp []map[string]interface{}
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
			names = append(names, gen.LangStringNameType{
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
//   - []gen.LangStringTextType: Slice of parsed language-specific text objects
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
func ParseLangStringTextType(descriptions json.RawMessage) ([]gen.LangStringTextType, error) {
	var texts []gen.LangStringTextType
	// remove id field from json
	var temp []map[string]interface{}
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
			texts = append(texts, gen.LangStringTextType{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

func ParseLangStringPreferredNameTypeIec61360(descriptions json.RawMessage) ([]gen.LangStringPreferredNameTypeIec61360, error) {
	var texts []gen.LangStringPreferredNameTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
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
			texts = append(texts, gen.LangStringPreferredNameTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

func ParseLangStringShortNameTypeIec61360(descriptions json.RawMessage) ([]gen.LangStringShortNameTypeIec61360, error) {
	var texts []gen.LangStringShortNameTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
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
			texts = append(texts, gen.LangStringShortNameTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

func ParseLangStringDefinitionTypeIec61360(descriptions json.RawMessage) ([]gen.LangStringDefinitionTypeIec61360, error) {
	var texts []gen.LangStringDefinitionTypeIec61360
	// remove id field from json
	var temp []map[string]interface{}
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
			texts = append(texts, gen.LangStringDefinitionTypeIec61360{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

func ParseQualifiersRow(row json.RawMessage) ([]QualifierRow, error) {
	var texts []QualifierRow
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling qualifier data: %w", err)
	}
	return texts, nil
}

func ParseExtensionRows(row json.RawMessage) ([]ExtensionRow, error) {
	var texts []ExtensionRow
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling extension data: %w", err)
	}
	return texts, nil
}

func ParseAdministrationRow(row json.RawMessage) (*AdministrationRow, error) {
	var texts []AdministrationRow
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling AdministrationRow data: %w", err)
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("no AdministrationRow found")
	}
	return &texts[0], nil
}
