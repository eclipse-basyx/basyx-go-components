/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import "fmt"

// Blob Type of SubmodelElement
type Blob struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Blob$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Value string `json:"value,omitempty"`

	ContentType string `json:"contentType"`
}

// Getters
//
//nolint:all
func (a Blob) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a Blob) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a Blob) GetCategory() string {
	return a.Category
}

//nolint:all
func (a Blob) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a Blob) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a Blob) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a Blob) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a Blob) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a Blob) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a Blob) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *Blob) SetModelType(v string) {
	a.ModelType = v
}

//nolint:all
func (a *Blob) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *Blob) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *Blob) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *Blob) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *Blob) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *Blob) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *Blob) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *Blob) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *Blob) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertBlobRequired checks if the required fields are not zero-ed
func AssertBlobRequired(obj Blob) error {
	elements := map[string]interface{}{
		"modelType":   obj.ModelType,
		"contentType": obj.ContentType,
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
	if obj.SemanticID != nil {
		if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
			return err
		}
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

// AssertBlobConstraints checks if the values respects the defined constraints
func AssertBlobConstraints(obj Blob) error {
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
	if obj.SemanticID != nil {
		if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
			return err
		}
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
	return nil
}

// ToValueOnly converts the Blob element to its Value Only representation.
// Returns a map with "value" (base64-encoded content) and "contentType" fields.
//
// Example output:
//
//	{
//	  "value": "SGVsbG8gV29ybGQ=",
//	  "contentType": "text/plain"
//	}
func (b *Blob) ToValueOnly() interface{} {
	return map[string]interface{}{
		"value":       b.Value,
		"contentType": b.ContentType,
	}
}

// UpdateFromValueOnly updates the Blob element from a Value Only representation.
// Expects a map with "value" and "contentType" fields.
//
// Parameters:
//   - value: a map[string]interface{} with "value" and "contentType" keys
//
// Returns an error if:
//   - value is not a map
//   - required fields are missing
//   - field types are invalid
func (b *Blob) UpdateFromValueOnly(value interface{}) error {
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for Blob: expected map, got %T", value)
	}

	if v, ok := valueMap["value"].(string); ok {
		b.Value = v
	}

	if ct, ok := valueMap["contentType"].(string); ok {
		b.ContentType = ct
	}

	return nil
}
