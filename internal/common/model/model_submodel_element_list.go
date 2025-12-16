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
func (sel *SubmodelElementList) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Value as []json.RawMessage
	type Alias SubmodelElementList
	aux := &struct {
		Value []json.RawMessage `json:"value,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(sel),
	}

	// Unmarshal into the temporary struct
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Value field manually
	if aux.Value != nil {
		sel.Value = make([]SubmodelElement, len(aux.Value))
		for i, rawElement := range aux.Value {
			element, err := UnmarshalSubmodelElement(rawElement)
			if err != nil {
				return fmt.Errorf("failed to unmarshal element at index %d: %w", i, err)
			}
			sel.Value[i] = element
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
	if obj.SemanticIdListElement != nil {
		if err := AssertReferenceRequired(*obj.SemanticIdListElement); err != nil {
			return err
		}
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
	if obj.SemanticIdListElement != nil {
		if err := AssertReferenceConstraints(*obj.SemanticIdListElement); err != nil {
			return err
		}
	}
	return nil
}
