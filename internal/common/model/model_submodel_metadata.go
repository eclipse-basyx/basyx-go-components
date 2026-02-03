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

import "github.com/FriedJannik/aas-go-sdk/types"

// SubmodelMetadata struct representing metadata of a Submodel.
type SubmodelMetadata struct {
	Extensions []types.IExtension `json:"extensions,omitempty"`

	//nolint:revive // The regex is auto-generated from the AAS specification and should not be changed.
	Category string `json:"category,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	//nolint:all
	IdShort string `json:"idShort,omitempty"`

	DisplayName []types.ILangStringNameType `json:"displayName,omitempty"`

	Description []types.ILangStringTextType `json:"description,omitempty"`

	ModelType types.ModelType `json:"modelType"`

	Administration types.IAdministrativeInformation `json:"administration,omitempty"`

	//nolint:revive // The regex is auto-generated from the AAS specification and should not be changed.
	ID string `json:"id" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	EmbeddedDataSpecifications []types.IEmbeddedDataSpecification `json:"embeddedDataSpecifications,omitempty"`

	Qualifiers []types.IQualifier `json:"qualifiers,omitempty"`

	SemanticID types.IReference `json:"semanticId,omitempty"`

	//nolint:all
	SupplementalSemanticIds []types.IReference `json:"supplementalSemanticIds,omitempty"`

	Kind types.ModellingKind `json:"kind,omitempty"`
}

// AssertSubmodelMetadataRequired checks if the required fields are not zero-ed
func AssertSubmodelMetadataRequired(obj SubmodelMetadata) error {
	elements := map[string]any{
		"modelType": obj.ModelType,
		"id":        obj.ID,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}
	return nil
}

// AssertSubmodelMetadataConstraints checks if the values respects the defined constraints
func AssertSubmodelMetadataConstraints(obj SubmodelMetadata) error {
	return nil
}
