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

// MultiLanguageProperty type of SubmodelElement
type MultiLanguageProperty struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^MultiLanguageProperty$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Value []LangStringTextType `json:"value,omitempty"`

	ValueID *Reference `json:"valueId,omitempty"`
}

// Getters
//
//nolint:all
func (a MultiLanguageProperty) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a MultiLanguageProperty) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a MultiLanguageProperty) GetCategory() string {
	return a.Category
}

//nolint:all
func (a MultiLanguageProperty) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a MultiLanguageProperty) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a MultiLanguageProperty) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a MultiLanguageProperty) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a MultiLanguageProperty) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a MultiLanguageProperty) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a MultiLanguageProperty) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (p *MultiLanguageProperty) SetModelType(modelType string) {
	p.ModelType = modelType
}

//nolint:all
func (a *MultiLanguageProperty) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *MultiLanguageProperty) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *MultiLanguageProperty) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *MultiLanguageProperty) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *MultiLanguageProperty) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *MultiLanguageProperty) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *MultiLanguageProperty) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *MultiLanguageProperty) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *MultiLanguageProperty) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertMultiLanguagePropertyRequired checks if the required fields are not zero-ed
func AssertMultiLanguagePropertyRequired(obj MultiLanguageProperty) error {
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
	for _, el := range obj.Value {
		if err := AssertLangStringTextTypeRequired(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceRequired(*obj.ValueID); err != nil {
		return err
	}
	return nil
}

// AssertMultiLanguagePropertyConstraints checks if the values respects the defined constraints
func AssertMultiLanguagePropertyConstraints(obj MultiLanguageProperty) error {
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
	for _, el := range obj.Value {
		if err := AssertLangStringTextTypeConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceConstraints(*obj.ValueID); err != nil {
		return err
	}
	return nil
}

// ToValueOnly converts the MultiLanguageProperty to its value-only representation.
// Returns an array of single-key objects: [{"en": "text"}, {"de": "Text"}]
// Returns an empty array if no language strings are present (to preserve array indices in lists).
func (m *MultiLanguageProperty) ToValueOnly() interface{} {
	result := make([]map[string]string, len(m.Value))
	for i, langString := range m.Value {
		result[i] = map[string]string{
			langString.Language: langString.Text,
		}
	}
	return result
}

// UpdateFromValueOnly updates the MultiLanguageProperty from a value-only representation.
// Expects an array of objects, each with a single language-text key-value pair.
// Example: [{"en": "Hello"}, {"de": "Hallo"}]
// Returns an error if the value type doesn't match the expected format.
func (m *MultiLanguageProperty) UpdateFromValueOnly(value interface{}) error {
	langArray, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for MultiLanguageProperty: expected array of objects, got %T", value)
	}

	langStrings := make([]LangStringTextType, 0, len(langArray))
	for _, langObj := range langArray {
		langMap, ok := langObj.(map[string]interface{})
		if !ok {
			continue
		}
		for lang, text := range langMap {
			if textStr, ok := text.(string); ok {
				langStrings = append(langStrings, LangStringTextType{
					Language: lang,
					Text:     textStr,
				})
			}
		}
	}
	m.Value = langStrings
	return nil
}
