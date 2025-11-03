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
)

// SubmodelElementCollection struct representing a collection of submodel elements.
type SubmodelElementCollection struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^SubmodelElementCollection$"`

	SemanticID *Reference `json:"semanticID,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Value []SubmodelElement `json:"value,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SubmodelElementCollection
func (sec *SubmodelElementCollection) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Value as []json.RawMessage
	type Alias SubmodelElementCollection
	aux := &struct {
		Value []json.RawMessage `json:"value,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(sec),
	}

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Value field manually
	if aux.Value != nil {
		sec.Value = make([]SubmodelElement, len(aux.Value))
		for i, rawElement := range aux.Value {
			element, err := UnmarshalSubmodelElement(rawElement)
			if err != nil {
				return fmt.Errorf("failed to unmarshal element at index %d: %w", i, err)
			}
			sec.Value[i] = element
		}
	}

	return nil
}

// Getters
//
//nolint:all
func (a SubmodelElementCollection) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a SubmodelElementCollection) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a SubmodelElementCollection) GetCategory() string {
	return a.Category
}

//nolint:all
func (a SubmodelElementCollection) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a SubmodelElementCollection) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a SubmodelElementCollection) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a SubmodelElementCollection) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a SubmodelElementCollection) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a SubmodelElementCollection) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a SubmodelElementCollection) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters
//
//nolint:all
func (a *SubmodelElementCollection) SetModelType(modelType string) {
	a.ModelType = modelType
}

//nolint:all
func (a *SubmodelElementCollection) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *SubmodelElementCollection) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *SubmodelElementCollection) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *SubmodelElementCollection) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *SubmodelElementCollection) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *SubmodelElementCollection) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *SubmodelElementCollection) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *SubmodelElementCollection) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *SubmodelElementCollection) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertSubmodelElementCollectionRequired checks if the required fields are not zero-ed
func AssertSubmodelElementCollectionRequired(obj SubmodelElementCollection) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
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
	return nil
}

// AssertSubmodelElementCollectionConstraints checks if the values respects the defined constraints
func AssertSubmodelElementCollectionConstraints(obj SubmodelElementCollection) error {
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
	// for _, el := range obj.Value {
	// 	if err := AssertSubmodelElementChoiceConstraints(el); err != nil {
	// 		return err
	// 	}
	// } TODO: REDO IF NECESSARY
	return nil
}
