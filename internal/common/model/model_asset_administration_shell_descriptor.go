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
	"errors"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	jsoniter "github.com/json-iterator/go"
)

type AssetAdministrationShellDescriptor struct {
	Description []types.ILangStringTextType `json:"description,omitempty"`

	DisplayName []types.ILangStringNameType `json:"displayName,omitempty"`

	Extensions []types.Extension `json:"extensions,omitempty"`

	Administration types.IAdministrativeInformation `json:"administration,omitempty"`

	AssetKind *types.AssetKind `json:"assetKind,omitempty"`

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

// UnmarshalJSON implements custom unmarshaling for AssetAdministrationShellDescriptor
// It handles the Fromsonable Unmarshalling of the aas-go-sdk types
func (obj *AssetAdministrationShellDescriptor) UnmarshalJSON(data []byte) error {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var jsonable map[string]any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return err
	}

	// Check for unknown fields
	allowedFields := map[string]bool{
		"description":         true,
		"displayName":         true,
		"extensions":          true,
		"administration":      true,
		"assetKind":           true,
		"assetType":           true,
		"endpoints":           true,
		"globalAssetId":       true,
		"idShort":             true,
		"id":                  true,
		"specificAssetIds":    true,
		"submodelDescriptors": true,
	}
	for key := range jsonable {
		if !allowedFields[key] {
			return errors.New("unknown field: " + key)
		}
	}

	// Description
	if descs, ok := jsonable["description"].([]any); ok {
		for _, desc := range descs {
			descMap, ok := desc.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: description is not a map")
			}
			var langString types.ILangStringTextType
			langString, err := jsonization.LangStringTextTypeFromJsonable(descMap)
			if err != nil {
				return err
			}
			obj.Description = append(obj.Description, langString)
		}
	}

	// Display Name
	if dns, ok := jsonable["displayName"].([]any); ok {
		for _, dn := range dns {
			dnMap, ok := dn.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: displayName is not a map")
			}
			var langString types.ILangStringNameType
			langString, err := jsonization.LangStringNameTypeFromJsonable(dnMap)
			if err != nil {
				return err
			}
			obj.DisplayName = append(obj.DisplayName, langString)
		}
	}

	// Extensions
	if exts, ok := jsonable["extensions"].([]any); ok {
		for _, ext := range exts {
			extMap, ok := ext.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: extension is not a map")
			}
			var extension types.IExtension
			extension, err := jsonization.ExtensionFromJsonable(extMap)
			if err != nil {
				return err
			}
			convExt, ok := extension.(*types.Extension)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: extension is not of type Extension")
			}
			if convExt == nil {
				return errors.New("AssetAdministrationShellDescriptor: extension conversion resulted in nil")
			}
			obj.Extensions = append(obj.Extensions, *convExt)
		}
	}

	// Administration
	if admin, ok := jsonable["administration"].(map[string]any); ok {
		var administration types.IAdministrativeInformation
		administration, err := jsonization.AdministrativeInformationFromJsonable(admin)
		if err != nil {
			return err
		}
		obj.Administration = administration
	}

	// Specific Asset IDs
	if sais, ok := jsonable["specificAssetIds"].([]any); ok {
		for _, sai := range sais {
			saiMap, ok := sai.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: specificAssetId is not a map")
			}
			var specificAssetID types.ISpecificAssetID
			specificAssetID, err := jsonization.SpecificAssetIDFromJsonable(saiMap)
			if err != nil {
				return err
			}
			obj.SpecificAssetIds = append(obj.SpecificAssetIds, specificAssetID)
		}
	}

	// Endpoints
	if endpoints, ok := jsonable["endpoints"].([]any); ok {
		for _, ep := range endpoints {
			epMap, ok := ep.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: endpoint is not a map")
			}
			var endpoint Endpoint
			endpointBytes, err := jsoniter.Marshal(epMap)
			if err != nil {
				return err
			}
			err = jsoniter.Unmarshal(endpointBytes, &endpoint)
			if err != nil {
				return err
			}
			obj.Endpoints = append(obj.Endpoints, endpoint)
		}
	}

	//Extensions
	for _, ext := range obj.Extensions {
		validationErrors := []string{}
		verification.Verify(&ext, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})

		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: Extensions verification failed: " + validationErrors[0])
		}
	}

	// Submodel Descriptors
	if smds, ok := jsonable["submodelDescriptors"].([]any); ok {
		for _, smd := range smds {
			smdMap, ok := smd.(map[string]any)
			if !ok {
				return errors.New("AssetAdministrationShellDescriptor: submodelDescriptor is not a map")
			}
			var submodelDescriptor SubmodelDescriptor
			smdBytes, err := jsoniter.Marshal(smdMap)
			if err != nil {
				return err
			}
			err = jsoniter.Unmarshal(smdBytes, &submodelDescriptor)
			if err != nil {
				return err
			}
			obj.SubmodelDescriptors = append(obj.SubmodelDescriptors, submodelDescriptor)
		}
	}

	// Handle other simple fields
	if assetKind, ok := jsonable["assetKind"].(string); ok {
		assetKind, ok := stringification.AssetKindFromString(assetKind)
		if !ok {
			return errors.New("failed to convert string to AssetKind")
		}
		obj.AssetKind = &assetKind
	}
	if assetType, ok := jsonable["assetType"].(string); ok {
		obj.AssetType = assetType
	}
	if globalAssetId, ok := jsonable["globalAssetId"].(string); ok {
		obj.GlobalAssetId = globalAssetId
	}
	if idShort, ok := jsonable["idShort"].(string); ok {
		obj.IdShort = idShort
	}
	if id, ok := jsonable["id"].(string); ok {
		obj.Id = id
	}

	// Verify Description
	var validationErrors []string
	for _, el := range obj.Description {
		verification.Verify(el, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})

		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: Description verification failed: " + validationErrors[0])
		}
	}

	// Verify DisplayName
	validationErrors = []string{}
	for _, el := range obj.DisplayName {
		verification.Verify(el, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})

		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: DisplayName verification failed: " + validationErrors[0])
		}
	}

	// Verify Extensions
	validationErrors = []string{}
	for _, el := range obj.Extensions {
		verification.Verify(&el, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})

		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: Extensions verification failed: " + validationErrors[0])
		}
	}

	// Verify SpecificAssetIds
	validationErrors = []string{}
	for _, el := range obj.SpecificAssetIds {
		verification.Verify(el, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})

		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: SpecificAssetIds verification failed: " + validationErrors[0])
		}
	}

	// Administration
	validationErrors = []string{}
	if obj.Administration != nil {
		verification.Verify(obj.Administration, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})
		if len(validationErrors) > 0 {
			return errors.New("AssetAdministrationShellDescriptor: Administration verification failed: " + validationErrors[0])
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

	var extensions []map[string]any
	// Extensions
	for _, ext := range obj.Extensions {
		extJsonable, err := jsonization.ToJsonable(&ext)
		if err != nil {
			return nil, err
		}
		if ret == nil {
			ret = make(map[string]any)
		}
		if _, exists := ret["extensions"]; !exists {
			ret["extensions"] = []map[string]any{}
		}
		extensions = append(extensions, extJsonable)
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
	if len(extensions) > 0 {
		ret["extensions"] = extensions
	}
	if administration != nil {
		ret["administration"] = administration
	}
	if obj.AssetKind != nil {
		assetKind, ok := stringification.AssetKindToString(*obj.AssetKind)
		if !ok {
			return nil, errors.New("failed to convert AssetKind to string")
		}
		ret["assetKind"] = assetKind
	}
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
	if len(submodelDescriptors) > 0 {
		ret["submodelDescriptors"] = submodelDescriptors
	}
	return ret, nil
}
