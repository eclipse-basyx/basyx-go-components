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

// File Type of SubmodelElement
type File struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^File$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Value string `json:"value,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ContentType string `json:"contentType"`
}

// Getters
//
//nolint:all
func (a File) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a File) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a File) GetCategory() string {
	return a.Category
}

//nolint:all
func (a File) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a File) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a File) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a File) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a File) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a File) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a File) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *File) SetModelType(modelType string) {
	a.ModelType = modelType
}

//nolint:all
func (a *File) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *File) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *File) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *File) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *File) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *File) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *File) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *File) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *File) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertFileRequired checks if the required fields are not zero-ed
func AssertFileRequired(obj File) error {
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

// AssertFileConstraints checks if the values respects the defined constraints
func AssertFileConstraints(obj File) error {
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

// ToValueOnly converts the File element to its Value Only representation.
// Returns a map with "value" (file path) and "contentType" fields.
//
// Example output:
//
//	{
//	  "value": "/path/to/file.pdf",
//	  "contentType": "application/pdf"
//	}
func (a *File) ToValueOnly() interface{} {
	return map[string]interface{}{
		"value":       a.Value,
		"contentType": a.ContentType,
	}
}

// UpdateFromValueOnly updates the File element from a Value Only representation.
// Expects a map with "value" and "contentType" fields.
//
// Parameters:
//   - value: a map[string]interface{} with "value" and "contentType" keys
//
// Returns an error if:
//   - value is not a map
//   - required fields are missing
//   - field types are invalid
func (a *File) UpdateFromValueOnly(value interface{}) error {
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for File: expected map, got %T", value)
	}

	if v, ok := valueMap["value"].(string); ok {
		a.Value = v
	}

	if ct, ok := valueMap["contentType"].(string); ok {
		a.ContentType = ct
	}

	return nil
}
