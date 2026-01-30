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
// Author: Martin Stemmer (Fraunhofer IESE), Jannik Fried (Fraunhofer IESE)

//nolint:all
package model

import (
	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

type AssetAdministrationShellDescriptor struct {
	Description []types.ILangStringTextType `json:"description,omitempty"`

	DisplayName []types.ILangStringNameType `json:"displayName,omitempty"`

	Extensions []types.Extension `json:"extensions,omitempty"`

	Administration types.IAdministrativeInformation `json:"administration,omitempty"`

	AssetKind types.AssetKind `json:"assetKind,omitempty"`

	AssetType string `json:"assetType,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Endpoints []Endpoint `json:"endpoints,omitempty"`

	GlobalAssetId string `json:"globalAssetId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	IdShort string `json:"idShort,omitempty" validate:"regexp=^[a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9_]+$"`

	Id string `json:"id" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	SpecificAssetIds []types.ISpecificAssetID `json:"specificAssetIds,omitempty"`

	SubmodelDescriptors []SubmodelDescriptor `json:"submodelDescriptors,omitempty"`
}

// AssertAssetAdministrationShellDescriptorRequired checks if the required fields are not zero-ed
func AssertAssetAdministrationShellDescriptorRequired(obj AssetAdministrationShellDescriptor) error {
	elements := map[string]any{
		"id": obj.Id,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Endpoints {
		if err := AssertEndpointRequired(el); err != nil {
			return err
		}
	}

	for _, el := range obj.SubmodelDescriptors {
		if err := AssertSubmodelDescriptorRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertAssetAdministrationShellDescriptorConstraints checks if the values respects the defined constraints
func AssertAssetAdministrationShellDescriptorConstraints(obj AssetAdministrationShellDescriptor) error {
	for _, el := range obj.Endpoints {
		if err := AssertEndpointConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.SubmodelDescriptors {
		if err := AssertSubmodelDescriptorConstraints(el); err != nil {
			return err
		}
	}
	return nil
}

func (obj AssetAdministrationShellDescriptor) ToJsonable() (map[string]any, error) {
	// Marshal every AAS GO SDK Type
	ret := make(map[string]any)
	// Description
	var descriptions []map[string]any
	for _, desc := range obj.Description {
		desc, err := jsonization.ToJsonable(desc)
		if err != nil {
			return nil, err
		}
		descriptions = append(descriptions, desc)
	}

	//Display Name
	var displayNames []map[string]any
	for _, dn := range obj.DisplayName {
		dn, err := jsonization.ToJsonable(dn)
		if err != nil {
			return nil, err
		}
		displayNames = append(displayNames, dn)
	}

	// Administration
	var administration map[string]any
	if obj.Administration != nil {
		var err error
		administration, err = jsonization.ToJsonable(obj.Administration)
		if err != nil {
			return nil, err
		}
	}

	// Specific Asset IDs
	var specificAssetIDs []map[string]any
	for _, sai := range obj.SpecificAssetIds {
		sai, err := jsonization.ToJsonable(sai)
		if err != nil {
			return nil, err
		}
		specificAssetIDs = append(specificAssetIDs, sai)
	}

	var submodelDescriptors []map[string]any
	//Submodel Descriptors
	for _, smd := range obj.SubmodelDescriptors {
		jsonable, err := smd.ToJsonable()
		if err != nil {
			return nil, err
		}
		if ret == nil {
			ret = make(map[string]any)
		}
		submodelDescriptors = append(submodelDescriptors, jsonable)
	}

	if len(descriptions) > 0 {
		ret["description"] = descriptions
	}
	if len(displayNames) > 0 {
		ret["displayName"] = displayNames
	}
	if len(obj.Extensions) > 0 {
		ret["extensions"] = obj.Extensions
	}
	if administration != nil {
		ret["administration"] = administration
	}
	ret["assetKind"] = obj.AssetKind
	if obj.AssetType != "" {
		ret["assetType"] = obj.AssetType
	}
	if len(obj.Endpoints) > 0 {
		ret["endpoints"] = obj.Endpoints
	}
	if obj.GlobalAssetId != "" {
		ret["globalAssetId"] = obj.GlobalAssetId
	}
	if obj.IdShort != "" {
		ret["idShort"] = obj.IdShort
	}
	if obj.Id != "" {
		ret["id"] = obj.Id
	}
	if len(specificAssetIDs) > 0 {
		ret["specificAssetIds"] = specificAssetIDs
	}
	return ret, nil
}
