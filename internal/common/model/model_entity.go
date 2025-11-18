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

// Entity type of Entity
type Entity struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Entity$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Statements []SubmodelElement `json:"statements,omitempty"`

	EntityType EntityType `json:"entityType"`

	GlobalAssetID string `json:"globalAssetId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	SpecificAssetIds []SpecificAssetID `json:"specificAssetIds,omitempty"`
}

// Getters
//
//nolint:all
func (a Entity) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a Entity) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a Entity) GetCategory() string {
	return a.Category
}

//nolint:all
func (a Entity) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a Entity) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a Entity) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a Entity) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a Entity) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a Entity) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a Entity) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *Entity) SetModelType(v string) {
	a.ModelType = v
}

//nolint:all
func (a *Entity) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *Entity) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *Entity) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *Entity) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *Entity) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *Entity) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *Entity) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *Entity) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *Entity) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// UnmarshalJSON custom unmarshaler for Entity to handle the Statements field
func (a *Entity) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Statements as json.RawMessage
	type Alias Entity
	aux := &struct {
		Statements []json.RawMessage `json:"statements,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	// Unmarshal into the temporary struct
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Statements field manually
	if aux.Statements != nil {
		statements := make([]SubmodelElement, len(aux.Statements))
		for i, raw := range aux.Statements {
			element, err := UnmarshalSubmodelElement(raw)
			if err != nil {
				return fmt.Errorf("failed to unmarshal Statements[%d]: %w", i, err)
			}
			statements[i] = element
		}
		a.Statements = statements
	}

	return nil
}

// AssertEntityRequired checks if the required fields are not zero-ed
func AssertEntityRequired(obj Entity) error {
	elements := map[string]interface{}{
		"modelType":  obj.ModelType,
		"entityType": obj.EntityType,
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
	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertEntityConstraints checks if the values respects the defined constraints
func AssertEntityConstraints(obj Entity) error {
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
	for _, el := range obj.SpecificAssetIds {
		if err := AssertSpecificAssetIdConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
