/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Operation  type of SubmodelElement
type Operation struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Operation$"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	InputVariables []OperationVariable `json:"inputVariables,omitempty"`

	OutputVariables []OperationVariable `json:"outputVariables,omitempty"`

	InoutputVariables []OperationVariable `json:"inoutputVariables,omitempty"`
}

// Getters
//
//nolint:all
func (a Operation) GetExtensions() []Extension {
	return a.Extensions
}

//nolint:all
func (a Operation) GetIdShort() string {
	return a.IdShort
}

//nolint:all
func (a Operation) GetCategory() string {
	return a.Category
}

//nolint:all
func (a Operation) GetDisplayName() []LangStringNameType {
	return a.DisplayName
}

//nolint:all
func (a Operation) GetDescription() []LangStringTextType {
	return a.Description
}

//nolint:all
func (a Operation) GetModelType() string {
	return a.ModelType
}

//nolint:all
func (a Operation) GetSemanticID() *Reference {
	return a.SemanticID
}

//nolint:all
func (a Operation) GetSupplementalSemanticIds() []Reference {
	return a.SupplementalSemanticIds
}

//nolint:all
func (a Operation) GetQualifiers() []Qualifier {
	return a.Qualifiers
}

//nolint:all
func (a Operation) GetEmbeddedDataSpecifications() []EmbeddedDataSpecification {
	return a.EmbeddedDataSpecifications
}

// Setters

//nolint:all
func (a *Operation) SetModelType(modelType string) {
	a.ModelType = modelType
}

//nolint:all
func (a *Operation) SetExtensions(v []Extension) {
	a.Extensions = v
}

//nolint:all
func (a *Operation) SetIdShort(v string) {
	a.IdShort = v
}

//nolint:all
func (a *Operation) SetCategory(v string) {
	a.Category = v
}

//nolint:all
func (a *Operation) SetDisplayName(v []LangStringNameType) {
	a.DisplayName = v
}

//nolint:all
func (a *Operation) SetDescription(v []LangStringTextType) {
	a.Description = v
}

//nolint:all
func (a *Operation) SetSemanticID(v *Reference) {
	a.SemanticID = v
}

//nolint:all
func (a *Operation) SetSupplementalSemanticIds(v []Reference) {
	a.SupplementalSemanticIds = v
}

//nolint:all
func (a *Operation) SetQualifiers(v []Qualifier) {
	a.Qualifiers = v
}

//nolint:all
func (a *Operation) SetEmbeddedDataSpecifications(v []EmbeddedDataSpecification) {
	a.EmbeddedDataSpecifications = v
}

// AssertOperationRequired checks if the required fields are not zero-ed
func AssertOperationRequired(obj Operation) error {
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
	for _, el := range obj.InputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.OutputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.InoutputVariables {
		if err := AssertOperationVariableRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertOperationConstraints checks if the values respects the defined constraints
func AssertOperationConstraints(obj Operation) error {
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
	for _, el := range obj.InputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.OutputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.InoutputVariables {
		if err := AssertOperationVariableConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
