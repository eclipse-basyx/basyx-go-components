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

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

type Property struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Property$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	ValueType DataTypeDefXsd `json:"valueType"`

	Value string `json:"value,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ValueID *Reference `json:"valueId,omitempty"`
}

// NewProperty creates a new Property instance
func NewProperty(valueType DataTypeDefXsd) *Property {
	return &Property{
		ValueType: valueType,
		ModelType: "Property",
	}
}

//nolint:all
func (p *Property) GetIdShort() string {
	return p.IdShort
}

//nolint:all
func (p *Property) GetCategory() string {
	return p.Category
}

//nolint:all
func (p *Property) GetDisplayName() []LangStringNameType {
	return p.DisplayName
}

//nolint:all
func (p *Property) GetDescription() []LangStringTextType {
	return p.Description
}

//nolint:all
func (p *Property) GetModelType() string {
	return p.ModelType
}

//nolint:all
func (p *Property) GetSemanticID() *Reference {
	return p.SemanticID
}

//nolint:all
func (p *Property) GetSupplementalSemanticIds() []Reference {
	return p.SupplementalSemanticIds
}

//nolint:all
func (p *Property) GetQualifiers() []Qualifier {
	return p.Qualifiers
}

//nolint:all
func (p *Property) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return p.EmbeddedDataSpecifications
}

//nolint:all
func (p *Property) GetExtensions() []Extension {
	return p.Extensions
}

//nolint:all
func (p *Property) GetValueType() DataTypeDefXsd {
	return p.ValueType
}

//nolint:all
func (p *Property) GetValue() string {
	return p.Value
}

//nolint:all
func (p *Property) GetValueID() *Reference {
	return p.ValueID
}

//nolint:all
func (p *Property) SetModelType(modelType string) {
	p.ModelType = modelType
}

//nolint:all
func (p *Property) SetIdShort(idShort string) {
	p.IdShort = idShort
}

//nolint:all
func (p *Property) SetCategory(category string) {
	p.Category = category
}

//nolint:all
func (p *Property) SetDisplayName(displayName []LangStringNameType) {
	p.DisplayName = displayName
}

//nolint:all
func (p *Property) SetDescription(description []LangStringTextType) {
	p.Description = description
}

//nolint:all
func (p *Property) SetSemanticID(semanticID *Reference) {
	p.SemanticID = semanticID
}

//nolint:all
func (p *Property) SetSupplementalSemanticIds(supplementalSemanticIds []Reference) {
	p.SupplementalSemanticIds = supplementalSemanticIds
}

//nolint:all
func (p *Property) SetQualifiers(qualifiers []Qualifier) {
	p.Qualifiers = qualifiers
}

//nolint:all
func (p *Property) SetEmbeddedDataSpecifications(embeddedDataSpecifications []EmbeddedDataSpecification) {
	p.EmbeddedDataSpecifications = embeddedDataSpecifications
}

//nolint:all
func (p *Property) SetExtensions(extensions []Extension) {
	p.Extensions = extensions
}

// AssertPropertyRequired checks if the required fields are not zero-ed
func AssertPropertyRequired(obj Property) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"valueType": obj.ValueType,
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
	if err := AssertStringConstraints(obj.IdShort); err != nil {
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
	if obj.ValueID != nil {
		if err := AssertReferenceRequired(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}

// AssertPropertyConstraints checks if the values respects the defined constraints
func AssertPropertyConstraints(obj Property) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertStringConstraints(obj.IdShort); err != nil {
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
	if obj.ValueID != nil {
		if err := AssertReferenceConstraints(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}
