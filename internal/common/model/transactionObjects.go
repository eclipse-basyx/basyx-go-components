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

package model

import (
	"database/sql"
	"encoding/json"
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
	Category sql.NullString
	// Kind specifies whether the submodel is a Template or Instance
	Kind string
	// EmbeddedDataSpecification contains embedded data specifications as JSON data
	EmbeddedDataSpecification json.RawMessage
	// SupplementalSemanticIDs contains supplemental semantic identifiers as JSON data
	SupplementalSemanticIDs json.RawMessage
	// Extensions contains extension as JSON data
	Extensions json.RawMessage
	// DisplayNames contains localized names as JSON data
	DisplayNames json.RawMessage
	// Descriptions contains localized descriptions as JSON data
	Descriptions json.RawMessage
	// SemanticID is a reference to a semantic definition as JSON data
	SemanticID json.RawMessage
	// ReferredSemanticIDs contains references to additional semantic definitions as JSON data
	ReferredSemanticIDs json.RawMessage
	// Qualifiers contains qualifier information as JSON data
	Qualifiers json.RawMessage
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
	DbID sql.NullInt64 `json:"db_id"`
	// ParentID is the database ID of the parent submodel element (nullable for root elements)
	ParentID sql.NullInt64 `json:"parent_id"`
	// RootID is the database ID of the root submodel element (nullable for root elements)
	RootID sql.NullInt64 `json:"root_id"`
	// IDShort is the short identifier for the submodel element
	IDShort string `json:"id_short"`
	// IDShortPath is the identifier path for the submodel element
	IDShortPath string `json:"id_short_path"`
	// DisplayNames contains localized names as JSON data
	DisplayNames *json.RawMessage `json:"displayNames,omitempty"`
	// Descriptions contains localized descriptions as JSON data
	Descriptions *json.RawMessage `json:"descriptions,omitempty"`
	// Category defines the category classification of the submodel element
	Category sql.NullString `json:"category"`
	// ModelType specifies the concrete type of the submodel element (e.g., Property, Operation, SubmodelElementCollection)
	ModelType string `json:"model_type"`
	// Value contains the actual value data of the submodel element as JSON data
	Value *json.RawMessage `json:"value"`
	// SemanticID is a reference to a semantic definition as JSON data
	SemanticID *json.RawMessage `json:"semanticId"`
	// SemanticIDReferred contains referred semantic ID references as JSON data
	SemanticIDReferred *json.RawMessage `json:"semanticIdReferred"`
	// SupplementalSemanticIDs contains supplemental semantic identifiers as JSON data
	SupplementalSemanticIDs *json.RawMessage `json:"supplementalSemanticIdReferenceRows"`
	// SupplementalSemanticIDsReferred contains referred supplemental semantic ID references as JSON data
	SupplementalSemanticIDsReferred *json.RawMessage `json:"supplementalSemanticIdReferredReferenceRows"`
	// Qualifiers contains qualifier information as JSON data
	Qualifiers *json.RawMessage `json:"qualifiers"`
	// Qualifiers contains qualifier information as JSON data
	Extensions *json.RawMessage `json:"extensions"`
	// Position specifies the position/order of the submodel element among its siblings
	Position int `json:"position"`
	// EmbeddedDataSpecifications contains embedded data specifications as JSON data
	EmbeddedDataSpecifications *json.RawMessage `json:"embeddedDataSpecifications"`
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
	ValueType DataTypeDefXsd `json:"value_type"`

	// ValueID contains value ID reference data as JSON data
	ValueID json.RawMessage `json:"value_id"`
	// ValueIDReferred contains referred value ID references as JSON data
	ValueIDReferred json.RawMessage `json:"value_id_referred"`
}

// BasicEventElementValueRow represents a data row for a BasicEventElement entity in the database.
// BasicEventElements are submodel elements that represent events occurring within the AAS environment.
//
// This structure captures the essential information about basic events, including their direction,
// state information, message topics for event communication, timing constraints, and references
// to observed elements that trigger the events.
type BasicEventElementValueRow struct {
	// Direction specifies the direction of the event (e.g., input, output)
	Direction string `json:"direction"`
	// State represents the current state of the event element
	State string `json:"state"`
	// MessageTopic is the topic used for event message communication
	MessageTopic string `json:"message_topic"`
	// LastUpdate contains the timestamp of the last event update
	LastUpdate string `json:"last_update"`
	// MinInterval specifies the minimum interval between event occurrences
	MinInterval string `json:"min_interval"`
	// MaxInternal specifies the maximum interval between event occurrences
	MaxInterval string `json:"max_interval"`
	// ObservedRef contains reference data to the observed element as JSON data
	ObservedRef json.RawMessage `json:"observed_ref"`
	// ObservedRefReferred contains referred reference data to the observed element as JSON data
	ObservedRefReferred json.RawMessage `json:"observed_ref_referred"`
	// ObservedRef contains reference data to the observed element as JSON data
	MessageBrokerRef json.RawMessage `json:"message_broker_ref"`
	// ObservedRefReferred contains referred reference data to the observed element as JSON data
	MessageBrokerRefReferred json.RawMessage `json:"message_broker_ref_referred"`
}

// SubmodelElementListRow represents a data row for a SubmodelElementList entity in the database.
// SubmodelElementLists are collections that contain multiple submodel elements of the same type.
//
// This structure defines the characteristics of a list collection, including whether the order
// of elements matters, the type constraints for list elements, and semantic information
// that applies to all elements within the list.
type SubmodelElementListRow struct {
	// OrderRelevant indicates whether the order of elements in the list is significant
	OrderRelevant bool `json:"order_relevant"`
	// TypeValueListElement specifies the required type for elements in the list
	TypeValueListElement string `json:"type_value_list_element"`
	// ValueTypeListElement specifies the required value type for elements in the list
	ValueTypeListElement string `json:"value_type_list_element"`
	// SemanticIDListElement contains semantic ID reference data for list elements as JSON data
	SemanticIDListElement json.RawMessage `json:"semantic_id_list_element"`
	// SemanticIDListElementReferred contains referred semantic ID reference data for list elements as JSON data
	SemanticIDListElementReferred json.RawMessage `json:"semantic_id_list_element_referred"`
}

// MultiLanguagePropertyValueRow represents a data row for a MultiLanguageProperty element's value in the database.
// MultiLanguageProperties are submodel elements that store localized text values in multiple languages.
//
// This structure captures the multilingual value data along with optional value ID references
// that provide semantic information about the property's value. The actual multilingual values
// are stored as JSON to accommodate the complex structure of language-text pairs.
type MultiLanguagePropertyValueRow struct {
	// ValueID contains value ID reference data as JSON data
	ValueID json.RawMessage `json:"value_id"`
	// ValueIDReferred contains referred value ID reference data as JSON data
	ValueIDReferred json.RawMessage `json:"value_id_referred"`
	// Value contains the multilingual text values as JSON data (array of language-text pairs)
	Value json.RawMessage `json:"value"`
}

// RangeValueRow represents a data row for a Range element's value in the database.
// Range elements define value intervals with minimum and maximum bounds of a specific data type.
//
// This structure captures the essential information of a range value, including the minimum
// and maximum bounds as strings and the data type that applies to both bounds. The bounds
// are stored as strings and must be interpreted according to the ValueType field.
type RangeValueRow struct {
	// Min is the minimum value of the range stored as a string.
	// The string representation must be interpreted according to the ValueType field.
	Min string `json:"min"`
	// Max is the maximum value of the range stored as a string.
	// The string representation must be interpreted according to the ValueType field.
	Max string `json:"max"`
	// ValueType specifies the XSD data type of both the Min and Max values.
	// This determines how the Min and Max strings should be parsed and interpreted
	// (e.g., xs:string, xs:int, xs:boolean, xs:dateTime, etc.).
	ValueType string `json:"value_type"`
}

// OperationValueRow represents a data row for an Operation element's variables in the database.
// Operations are submodel elements that define executable functions with input and output variables.
//
// This structure captures the input, output, and in-output operation variables as JSON data.
// Each variable is represented as a collection of OperationVariable objects, allowing for
// complex definitions of operation parameters.
type OperationValueRow struct {
	// InputVariables contains input operation variables as JSON data
	InputVariables json.RawMessage `json:"input_variables"`
	// OutputVariables contains output operation variables as JSON data
	OutputVariables json.RawMessage `json:"output_variables"`
	// InoutputVariables contains in-output operation variables as JSON data
	InoutputVariables json.RawMessage `json:"inoutput_variables"`
}

// EntityValueRow represents a data row for an Entity element in the database.
// Entities are submodel elements that represent real-world objects or concepts within the AAS.
//
// This structure captures the essential information of an entity, including its type,
// global asset identifier, and associated statements that describe the entity's characteristics.
// The statements are stored as JSON to accommodate complex structures.
type EntityValueRow struct {
	// EntityType specifies the type of the entity.
	EntityType string `json:"entity_type"`
	// GlobalAssetID specifies the global asset identifier of the entity.
	GlobalAssetID string `json:"global_asset_id"`
	// SpecificAssetIDs contains specific asset ID references as JSON data
	SpecificAssetIDs json.RawMessage `json:"specific_asset_ids"`
}

// RelationshipElementValueRow represents a data row for a ReferenceElement entity in the database.
// ReferenceElements are submodel elements that encapsulate references to other AAS elements.
//
// This structure captures the two references that make up a ReferenceElement.
// Each reference is stored as JSON data to accommodate the complex structure of references.
type RelationshipElementValueRow struct {
	// First contains the first reference as JSON data
	First json.RawMessage `json:"first"`
	// Second contains the second reference as JSON data
	Second json.RawMessage `json:"second"`
}

// AnnotatedRelationshipElementValueRow represents a data row for an AnnotatedRelationshipElement entity in the database.
// AnnotatedRelationshipElements are submodel elements that define relationships between two references,
// along with additional annotations that provide context or metadata about the relationship.
//
// This structure captures the two references that make up the relationship and the associated annotations.
// Each component is stored as JSON data to accommodate complex structures.
type AnnotatedRelationshipElementValueRow struct {
	// First contains the first reference as JSON data
	First json.RawMessage `json:"first"`
	// Second contains the second reference as JSON data
	Second json.RawMessage `json:"second"`
}

// ReferenceElementValueRow represents a data row for a ReferenceElement entity's value in the database.
// ReferenceElements are submodel elements that encapsulate a single reference to other AAS elements.
//
// This structure captures the reference value of a ReferenceElement.
// The reference is stored as JSON data to accommodate the complex structure of references.
type ReferenceElementValueRow struct {
	// Value contains the reference as JSON data
	Value json.RawMessage `json:"value"`
}

// FileElementValueRow represents a data row for a FileElement entity in the database.
// FileElements are submodel elements that represent files associated with the AAS.
//
// This structure captures the file path and content type of a FileElement.
// The file path is stored as a string, while the content type specifies the MIME type of the file.
type FileElementValueRow struct {
	// Value contains the file path (relative or absolute) as a string
	Value string `json:"value"`
	// ContentType specifies the MIME type of the file content
	ContentType string `json:"content_type"`
}

// MultiLanguagePropertyElementValueRow represents a data row for a MultiLanguageProperty entity in the database.
// MultiLanguageProperties are submodel elements that represent text values in multiple languages.
//
// This structure captures the language-text pairs and optional value ID reference.
type MultiLanguagePropertyElementValueRow struct {
	// Value contains the array of language-text pairs
	Value []LangStringTextType `json:"value"`
	// ValueID is a reference to a related concept or value definition (optional)
	ValueID *json.RawMessage `json:"value_id"`
	// ValueIDReferred contains the referred semantic ID for the value reference (optional)
	ValueIDReferred *json.RawMessage `json:"value_id_referred"`
}

// BlobElementValueRow represents a data row for a BlobElement entity in the database.
// BlobElements are submodel elements that represent binary large objects (BLOBs) associated with the AAS.
//
// This structure captures the content type and binary value of a BlobElement.
// The content type specifies the MIME type of the blob, while the value contains the actual binary data.
type BlobElementValueRow struct {
	// ContentType specifies the MIME type of the blob content
	ContentType string `json:"content_type"`
	// Value contains the blob data as a byte array
	Value string `json:"value"`
}

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
