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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
)

// SubmodelRow represents a row from the Submodel table in the database.
// JSON fields are represented as json.RawMessage to allow for deferred parsing
// and handling of complex nested structures.
//
// This structure is used when retrieving submodel data from SQL queries,
// where complex fields like DisplayNames, Descriptions, and References are
// stored as JSON in the database and need to be parsed separately.
type SubmodelRow struct {
	// ID is the unique identifier of the submodel
	ID string
	// IDShort is the short identifier for the submodel
	IDShort string
	// Category defines the category classification of the submodel
	Category string
	// Kind specifies whether the submodel is a Template or Instance
	Kind string
	// EmbeddedDataSpecification contains embedded data specifications as JSON data
	EmbeddedDataSpecification json.RawMessage
	// DisplayNames contains localized names as JSON data
	DisplayNames json.RawMessage
	// Descriptions contains localized descriptions as JSON data
	Descriptions json.RawMessage
	// SemanticID is a reference to a semantic definition as JSON data
	SemanticID json.RawMessage
	// ReferredSemanticIDs contains references to additional semantic definitions as JSON data
	ReferredSemanticIDs json.RawMessage
	// SupplementalSemanticIDs contains supplemental semantic identifiers as JSON data
	SupplementalSemanticIDs json.RawMessage
	// SupplementalReferredSemIDs contains referred supplemental semantic identifiers as JSON data
	SupplementalReferredSemIDs json.RawMessage
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
// with the same ReferenceID and ReferenceType but different Key details.
type ReferenceRow struct {
	// ReferenceID is the unique identifier of the reference in the database
	ReferenceID int64 `json:"reference_id"`
	// ReferenceType specifies the type of reference (e.g., ExternalReference, ModelReference)
	ReferenceType string `json:"reference_type"`
	// KeyID is the unique identifier of the key in the database (nullable)
	KeyID *int64 `json:"key_id"`
	// KeyType specifies the type of the key (e.g., Submodel, Property) (nullable)
	KeyType *string `json:"key_type"`
	// KeyValue contains the actual value of the key (nullable)
	KeyValue *string `json:"key_value"`
}

// EdsReferenceRow represents a data row for an embedded data specification reference entity in the database.
// This structure is used to store references associated with embedded data specifications (EDS).
//
// Each row contains information about a single key within a reference that is part of an
// embedded data specification. Multiple rows with the same EdsID and ReferenceID are aggregated
// to form complete reference objects.
type EdsReferenceRow struct {
	// EdsID is the unique identifier of the embedded data specification in the database
	EdsID int64 `json:"eds_id"`
	// ReferenceID is the unique identifier of the reference in the database
	ReferenceID int64 `json:"reference_id"`
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
// entries with the same ReferenceID and ReferenceType but different Key details.
type ReferredReferenceRow struct {
	// SupplementalRootReferenceID identifies the root supplemental reference (nullable)
	SupplementalRootReferenceID *int64 `json:"supplemental_root_reference_id"`
	// ReferenceID is the unique identifier of this reference in the database (nullable)
	ReferenceID *int64 `json:"reference_id"`
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

// EdsContentIec61360Row represents a data row for IEC 61360 data specification content.
// IEC 61360 is an international standard for representing data element types with semantic descriptions.
//
// This structure contains all the information needed to represent a data element according to
// IEC 61360, including preferred names, definitions, value formats, and unit references.
// Many fields are stored as JSON to handle complex nested structures like language strings and references.
type EdsContentIec61360Row struct {
	// EdsID is the unique identifier of the embedded data specification
	EdsID int64 `json:"eds_id"`
	// IecID is the unique identifier of the IEC 61360 content in the database
	IecID int64 `json:"iec_id"`
	// Unit specifies the unit of measurement for the data element
	Unit string `json:"unit"`
	// Position specifies the position of the IEC 61360 entry
	Position int `json:"position"`
	// SourceOfDefinition identifies where the definition comes from
	SourceOfDefinition string `json:"source_of_definition"`
	// Symbol is the symbolic representation of the data element
	Symbol string `json:"symbol"`
	// DataType specifies the data type of the value (e.g., STRING, INTEGER)
	DataType string `json:"data_type"`
	// ValueFormat describes the format of the value (e.g., date format, number format)
	ValueFormat string `json:"value_format"`
	// Value is the actual value of the data element
	Value string `json:"value"`
	// LevelType contains IEC level type information as JSON data
	LevelType json.RawMessage `json:"level_type"`
	// PreferredName contains localized preferred names as JSON data
	PreferredName json.RawMessage `json:"preferred_name"`
	// ShortName contains localized short names as JSON data
	ShortName json.RawMessage `json:"short_name"`
	// Definition contains localized definitions as JSON data
	Definition json.RawMessage `json:"definition"`
	// UnitReferenceKeys contains reference keys for the unit as JSON data
	UnitReferenceKeys json.RawMessage `json:"unit_reference_keys"`
	// UnitReferenceReferred contains referred unit references as JSON data
	UnitReferenceReferred json.RawMessage `json:"unit_reference_referred"`
	// ValueListEntries contains value list entries as JSON data
	ValueListEntries json.RawMessage `json:"value_list_entries"`
}

// ValueListRow represents a data row for value list entries in IEC 61360 data specifications.
// Value lists define enumerated values with their associated references and semantic meanings.
//
// This structure is used when a data element can only take on specific predefined values,
// each potentially having its own semantic reference explaining its meaning.
type ValueListRow struct {
	// Value is the actual value in the value list entry
	Value string `json:"value_pair_value"`
	// ValueRefPairID is the unique identifier of the value reference pair in the database
	ValueRefPairID int64 `json:"value_reference_pair_id"`
	// ReferenceRows contains reference data associated with this value as JSON data
	ReferenceRows json.RawMessage `json:"reference_rows"`
	// ReferredReferenceRows contains referred reference data associated with this value as JSON data
	ReferredReferenceRows json.RawMessage `json:"referred_reference_rows"`
}

// QualifierRow represents a data row for a Qualifier entity in the database.
// Qualifiers are additional characteristics that affect the value or interpretation of an element.
//
// In the AAS metamodel, qualifiers provide a way to add metadata or constraints to elements,
// such as value constraints, multiplicity, or semantic refinements. They include references
// to semantic IDs that define the meaning of the qualifier and value IDs that provide
// semantic information about the qualifier's value.
type QualifierRow struct {
	// DbID is the unique identifier of the qualifier in the database
	DbID int64 `json:"dbId"`
	// Kind specifies the kind of qualifier (e.g., ConceptQualifier, ValueQualifier)
	Kind string `json:"kind"`
	// Type is the type/name of the qualifier
	Type string `json:"type"`
	// Position specifies the position of the qualifier
	Position int `json:"position"`
	// ValueType specifies the data type of the qualifier value
	ValueType string `json:"value_type"`
	// Value is the actual value of the qualifier
	Value string `json:"value"`
	// SemanticID contains semantic ID reference data as JSON data
	SemanticID json.RawMessage `json:"semanticIdReferenceRows"`
	// SemanticIDReferredReferences contains referred semantic ID references as JSON data
	SemanticIDReferredReferences json.RawMessage `json:"semanticIdReferredReferencesRows"`
	// ValueID contains value ID reference data as JSON data
	ValueID json.RawMessage `json:"valueIdReferenceRows"`
	// ValueIDReferredReferences contains referred value ID references as JSON data
	ValueIDReferredReferences json.RawMessage `json:"valueIdReferredReferencesRows"`
	// SupplementalSemanticIDs contains supplemental semantic ID references as JSON data
	SupplementalSemanticIDs json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	// SupplementalSemanticIDsReferredReferences contains referred supplemental semantic ID references as JSON data
	SupplementalSemanticIDsReferredReferences json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
}

// ExtensionRow represents a data row for an Extension entity in the database.
// Extensions provide a way to add custom information to AAS elements beyond the standard metamodel.
//
// Extensions allow users to attach additional metadata or properties to elements that are not
// covered by the standard AAS specification. They include semantic references to define the
// meaning of the extension and can refer to other elements in the AAS.
type ExtensionRow struct {
	// DbID is the unique identifier of the extension in the database
	DbID int64 `json:"dbId"`
	// Position specifies the position of the extension
	Position int `json:"position"`
	// Name is the name of the extension
	Name string `json:"name"`
	// ValueType specifies the data type of the extension value
	ValueType string `json:"value_type"`
	// Value is the actual value of the extension
	Value string `json:"value"`
	// SemanticID contains semantic ID reference data as JSON data
	SemanticID json.RawMessage `json:"semanticIdReferenceRows"`
	// SemanticIDReferredReferences contains referred semantic ID references as JSON data
	SemanticIDReferredReferences json.RawMessage `json:"semanticIdReferredReferencesRows"`
	// SupplementalSemanticIDs contains supplemental semantic ID references as JSON data
	SupplementalSemanticIDs json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	// SupplementalSemanticIDsReferredReferences contains referred supplemental semantic ID references as JSON data
	SupplementalSemanticIDsReferredReferences json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
	// RefersTo contains references to other elements as JSON data
	RefersTo json.RawMessage `json:"refersToReferenceRows"`
	// RefersToReferredReferences contains referred references to other elements as JSON data
	RefersToReferredReferences json.RawMessage `json:"refersToReferredReferencesRows"`
}

// AdministrationRow represents a data row for administrative information in the database.
// Administrative information includes version control, revision tracking, and data specifications.
//
// This structure captures metadata about the lifecycle and provenance of AAS elements,
// including version numbers, revision information, creator references, and associated
// data specifications that define the element's structure and semantics.
type AdministrationRow struct {
	// DbID is the unique identifier of the administration record in the database
	DbID int64 `json:"dbId"`
	// Version is the version number of the element
	Version string `json:"version"`
	// Revision is the revision number of the element
	Revision string `json:"revision"`
	// TemplateID is the identifier of the template this element is based on
	TemplateID string `json:"templateId"`
	// EmbeddedDataSpecification contains embedded data specifications as JSON data
	EmbeddedDataSpecification json.RawMessage `json:"embedded_data_specification"`
	// Creator contains creator reference data as JSON data
	Creator json.RawMessage `json:"creator"`
	// CreatorReferred contains referred creator references as JSON data
	CreatorReferred json.RawMessage `json:"creatorReferred"`
}

// SubmodelElementRow represents a row from the SubmodelElement table in the database.
// Submodel elements are the actual data carriers within a submodel, representing properties,
// operations, collections, and other structural elements.
//
// This structure supports hierarchical submodel elements where elements can contain child elements.
// The ParentID field establishes the parent-child relationship, while Position determines the
// order of elements at the same level.
type SubmodelElementRow struct {
	// DbID is the unique identifier of the submodel element in the database
	DbID int64 `json:"db_id"`
	// ParentID is the database ID of the parent submodel element (nullable for root elements)
	ParentID *int64 `json:"parent_id"`
	// RootID is the database ID of the root submodel element (nullable for root elements)
	RootID *int64 `json:"root_id"`
	// IDShort is the short identifier for the submodel element
	IDShort string `json:"id_short"`
	// DisplayNames contains localized names as JSON data
	DisplayNames json.RawMessage `json:"displayNames"`
	// Descriptions contains localized descriptions as JSON data
	Descriptions json.RawMessage `json:"descriptions"`
	// Category defines the category classification of the submodel element
	Category string `json:"category"`
	// ModelType specifies the concrete type of the submodel element (e.g., Property, Operation, SubmodelElementCollection)
	ModelType string `json:"model_type"`
	// Value contains the actual value data of the submodel element as JSON data
	Value json.RawMessage `json:"value"`
	// SemanticID is a reference to a semantic definition as JSON data
	SemanticID json.RawMessage `json:"semanticId"`
	// SemanticIDReferred contains referred semantic ID references as JSON data
	SemanticIDReferred json.RawMessage `json:"semanticIdReferred"`
	// SupplementalSemanticIDs contains supplemental semantic identifiers as JSON data
	SupplementalSemanticIDs json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	// SupplementalSemanticIDsReferred contains referred supplemental semantic ID references as JSON data
	SupplementalSemanticIDsReferred json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
	// Qualifiers contains qualifier information as JSON data
	Qualifiers json.RawMessage `json:"qualifiers"`
	// Position specifies the position/order of the submodel element among its siblings
	Position int `json:"position"`
	// EmbeddedDataSpecifications contains embedded data specifications as JSON data
	EmbeddedDataSpecifications json.RawMessage `json:"embeddedDataSpecifications"`
}

// PropertyValueRow represents a data row for a Property element's value in the database.
// Properties are fundamental data-carrying elements in AAS that store typed values.
//
// This structure captures the essential information of a property value, including the
// actual value as a string and its data type according to XSD (XML Schema Definition) standards.
// The value is stored as a string in the database and must be interpreted according to its ValueType.
type PropertyValueRow struct {
	// Value is the actual value of the property stored as a string.
	// The string representation must be interpreted according to the ValueType field.
	Value string `json:"value"`

	// ValueType specifies the XSD data type of the value.
	// This determines how the Value string should be parsed and interpreted
	// (e.g., xs:string, xs:int, xs:boolean, xs:dateTime, etc.).
	ValueType model.DataTypeDefXsd `json:"value_type"`

	// ValueID contains value ID reference data as JSON data
	ValueID json.RawMessage `json:"value_id"`
	// ValueIDReferred contains referred value ID references as JSON data
	ValueIDReferred json.RawMessage `json:"value_id_referred"`
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
//   - Skips entries with nil RootReference, ReferenceID, ParentReference, or ReferenceType
//   - Verifies parent references exist in the builder map
//   - Ensures key data (KeyID, KeyType, KeyValue) is complete
func ParseReferredReferencesFromRows(semanticIDData []ReferredReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder) error {
	for _, ref := range semanticIDData {
		if ref.RootReference == nil {
			fmt.Println("[WARNING - ParseReferredReferencesFromRows] RootReference was nil - skipping Reference Creation.")
			continue
		}
		builder, semanticIDCreated := referenceBuilderRefs[*ref.RootReference]
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
//
// Returns:
//   - error: An error if JSON unmarshalling fails or if a parent reference is not found in the map.
//     Nil references or keys are logged as warnings but do not cause the function to fail.
func ParseReferredReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder) error {
	if len(row) == 0 {
		return nil
	}

	var semanticIDData []ReferredReferenceRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &semanticIDData); err != nil {
		return fmt.Errorf("error unmarshalling referred semantic ID data: %w", err)
	}

	return ParseReferredReferencesFromRows(semanticIDData, referenceBuilderRefs)
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
//
// Returns:
//   - []*model.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//
// The function:
//   - Groups multiple rows with the same ReferenceID into a single Reference
//   - Creates new ReferenceBuilder instances for each unique ReferenceId
//   - Validates key data completeness (KeyID, KeyType, KeyValue)
//   - Returns only the unique references (one per ReferenceID)
func ParseReferencesFromRows(semanticIDData []ReferenceRow, referenceBuilderRefs map[int64]*ReferenceBuilder) []*model.Reference {
	resultArray := make([]*model.Reference, 0)

	for _, ref := range semanticIDData {
		var semanticID *model.Reference
		var semanticIDBuilder *ReferenceBuilder

		_, semanticIDCreated := referenceBuilderRefs[ref.ReferenceID]

		if !semanticIDCreated {
			semanticID, semanticIDBuilder = NewReferenceBuilder(ref.ReferenceType, ref.ReferenceID)
			referenceBuilderRefs[ref.ReferenceID] = semanticIDBuilder
			resultArray = append(resultArray, semanticID)
		} else {
			semanticIDBuilder = referenceBuilderRefs[ref.ReferenceID]
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
//
// Returns:
//   - []*model.Reference: Slice of parsed Reference objects. Each Reference contains all its associated Keys.
//   - error: An error if JSON unmarshalling fails. Nil key data is logged as warnings but does not cause failure.
func ParseReferences(row json.RawMessage, referenceBuilderRefs map[int64]*ReferenceBuilder) ([]*model.Reference, error) {
	if len(row) == 0 {
		return make([]*model.Reference, 0), nil
	}

	var semanticIDData []ReferenceRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &semanticIDData); err != nil {
		return nil, fmt.Errorf("error unmarshalling semantic ID data: %w", err)
	}

	return ParseReferencesFromRows(semanticIDData, referenceBuilderRefs), nil
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
func ParseQualifiersRow(row json.RawMessage) ([]QualifierRow, error) {
	var texts []QualifierRow
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
func ParseExtensionRows(row json.RawMessage) ([]ExtensionRow, error) {
	var texts []ExtensionRow
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
func ParseAdministrationRow(row json.RawMessage) (*AdministrationRow, error) {
	var texts []AdministrationRow
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(row, &texts); err != nil {
		return nil, fmt.Errorf("error unmarshalling AdministrationRow data: %w", err)
	}
	if len(texts) == 0 {
		return nil, nil
	}
	return &texts[0], nil
}
