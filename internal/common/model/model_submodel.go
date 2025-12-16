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

	jsoniter "github.com/json-iterator/go"
)

// Submodel struct representing a Submodel.
type Submodel struct {
	Extensions []Extension `json:"extensions,omitempty"`

	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []LangStringNameType `json:"displayName,omitempty"`

	Description []LangStringTextType `json:"description,omitempty"`

	ModelType string `json:"modelType" validate:"regexp=^Submodel$"`

	Administration *AdministrativeInformation `json:"administration,omitempty"`

	ID string `json:"id" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Kind ModellingKind `json:"kind,omitempty"`

	SemanticID *Reference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []*Reference `json:"supplementalSemanticIds,omitempty"`

	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	SubmodelElements []SubmodelElement `json:"submodelElements,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Submodel to handle polymorphic SubmodelElements
func (s *Submodel) UnmarshalJSON(data []byte) error {
	type Alias Submodel
	aux := &struct {
		SubmodelElements           []json.RawMessage `json:"submodelElements,omitempty"`
		EmbeddedDataSpecifications []json.RawMessage `json:"embeddedDataSpecifications,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.SubmodelElements = make([]SubmodelElement, len(aux.SubmodelElements))
	for i, raw := range aux.SubmodelElements {
		elem, err := UnmarshalSubmodelElement(raw)
		if err != nil {
			return err
		}
		s.SubmodelElements[i] = elem
	}

	s.EmbeddedDataSpecifications = make([]EmbeddedDataSpecification, len(aux.EmbeddedDataSpecifications))
	for i, raw := range aux.EmbeddedDataSpecifications {
		var eds EmbeddedDataSpecification
		if err := json.Unmarshal(raw, &eds); err != nil {
			return err
		}
		s.EmbeddedDataSpecifications[i] = eds
	}

	return nil
}

// AssertSubmodelRequired checks if the required fields are not zero-ed
func AssertSubmodelRequired(obj Submodel) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
		"id":        obj.ID,
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
	if obj.DisplayName != nil {
		for _, el := range obj.DisplayName {
			if err := AssertLangStringNameTypeRequired(el); err != nil {
				return err
			}
		}
	}
	if obj.Description != nil {
		for _, el := range obj.Description {
			if err := AssertLangStringTextTypeRequired(el); err != nil {
				return err
			}
		}
	}
	if obj.Administration != nil {
		if err := AssertAdministrativeInformationRequired(*obj.Administration); err != nil {
			return err
		}
	}
	if obj.SemanticID != nil {
		if err := AssertReferenceRequired(*obj.SemanticID); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if el != nil {
			if err := AssertReferenceRequired(*el); err != nil {
				return err
			}
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

// AssertSubmodelConstraints checks if the values respects the defined constraints
func AssertSubmodelConstraints(obj Submodel) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	if err := AssertStringConstraints(obj.IdShort); err != nil {
		return err
	}
	if obj.DisplayName != nil {
		for _, el := range obj.DisplayName {
			if err := AssertLangStringNameTypeConstraints(el); err != nil {
				return err
			}
		}
	}
	if obj.Description != nil {
		for _, el := range obj.Description {
			if err := AssertLangStringTextTypeConstraints(el); err != nil {
				return err
			}
		}
	}
	if obj.Administration != nil {
		if err := AssertAdministrativeInformationConstraints(*obj.Administration); err != nil {
			return err
		}
	}
	if obj.SemanticID != nil {
		if err := AssertReferenceConstraints(*obj.SemanticID); err != nil {
			return err
		}
	}
	for _, el := range obj.SupplementalSemanticIds {
		if el != nil {
			if err := AssertReferenceConstraints(*el); err != nil {
				return err
			}
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
