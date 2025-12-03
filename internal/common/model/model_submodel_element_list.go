/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import (
	"encoding/json"
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// SubmodelElementList struct representing a SubmodelElementList.
type SubmodelElementList struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^SubmodelElementList$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	OrderRelevant bool `json:"orderRelevant,omitempty"`

	//nolint:all
	SemanticIdListElement *Reference `json:"semanticIdListElement,omitempty"`

	TypeValueListElement *AasSubmodelElements `json:"typeValueListElement,omitempty"`

	ValueTypeListElement DataTypeDefXsd `json:"valueTypeListElement,omitempty"`

	Value []SubmodelElement `json:"value,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SubmodelElementList
func (a *SubmodelElementList) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Value as []json.RawMessage
	type Alias SubmodelElementList
	aux := &struct {
		Value []json.RawMessage `json:"value,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	// Unmarshal into the temporary struct
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Value field manually
	if aux.Value != nil {
		a.Value = make([]SubmodelElement, len(aux.Value))
		for i, rawElement := range aux.Value {
			element, err := UnmarshalSubmodelElement(rawElement)
			if err != nil {
				return fmt.Errorf("failed to unmarshal element at index %d: %w", i, err)
			}
			a.Value[i] = element
		}
	}

	return nil
}

// Getters
//
//nolint:all
func (a SubmodelElementList) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a SubmodelElementList) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a SubmodelElementList) GetCategory() string {
	return a.Category
}

//nolint:all
func (a SubmodelElementList) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a SubmodelElementList) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a SubmodelElementList) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a SubmodelElementList) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a SubmodelElementList) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a SubmodelElementList) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a SubmodelElementList) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters
//
//nolint:all
func (a *SubmodelElementList) SetModelType(modelType string) {
	a.ModelType = modelType
}

//nolint:all
func (a *SubmodelElementList) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *SubmodelElementList) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *SubmodelElementList) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *SubmodelElementList) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *SubmodelElementList) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *SubmodelElementList) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *SubmodelElementList) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *SubmodelElementList) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *SubmodelElementList) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertSubmodelElementListRequired checks if the required fields are not zero-ed
func AssertSubmodelElementListRequired(obj SubmodelElementList) error {
	elements := map[string]interface{}{
		"modelType":            obj.ModelType,
		"typeValueListElement": obj.TypeValueListElement,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Extensions {
		if err := AssertExtensionRequired(el); err != nil {
			return err
		}
	}
	if err := AssertIdShortRequired(obj.IdShort); err != nil {
		return err
	}
	for _, el := range obj.DisplayName {
		if err := AssertLangStringNameTypeRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Description {
		if err := AssertLangStringTextTypeRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Qualifiers {
		if err := AssertQualifierRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceRequired(*obj.SemanticIdListElement); err != nil {
		return err
	}
	return nil
}

// AssertSubmodelElementListConstraints checks if the values respects the defined constraints
func AssertSubmodelElementListConstraints(obj SubmodelElementList) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertstringConstraints(obj.IdShort); err != nil {
		return err
	}
	for _, el := range obj.DisplayName {
		if err := AssertLangStringNameTypeConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Description {
		if err := AssertLangStringTextTypeConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
		return err
	}
	for _, el := range obj.SupplementalSemanticIds {
		if err := AssertReferenceConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Qualifiers {
		if err := AssertQualifierConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EmbeddedDataSpecifications {
		if err := AssertEmbeddedDataSpecificationConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceConstraints(*obj.SemanticIdListElement); err != nil {
		return err
	}
	return nil
}

// ToValueOnly converts the SubmodelElementList to its Value Only representation.
// Returns an array of value-only representations of the child elements.
// Returns nil if the list has no elements.
//
// Parameters:
//   - elementSerializer: function to convert child SubmodelElements to value-only form
//
// Example output:
//
//	[
//	  "value1",
//	  {...},
//	  ...
//	]
func (a *SubmodelElementList) ToValueOnly(elementSerializer func([]SubmodelElement) interface{}) interface{} {
	if len(a.Value) == 0 {
		return nil
	}
	return elementSerializer(a.Value)
}

// UpdateFromValueOnly updates the SubmodelElementList from a Value Only representation.
// Expects an array of value-only element representations.
//
// Parameters:
//   - value: the value-only representation (typically an array)
//   - elementDeserializer: function to convert value-only form to SubmodelElement slice
//
// Returns an error if deserialization fails.
func (a *SubmodelElementList) UpdateFromValueOnly(
	value interface{},
	elementDeserializer func(interface{}) ([]SubmodelElement, error),
) error {
	if value == nil {
		a.Value = nil
		return nil
	}

	elements, err := elementDeserializer(value)
	if err != nil {
		return fmt.Errorf("failed to deserialize SubmodelElementList value: %w", err)
	}

	a.Value = elements
	return nil
}
