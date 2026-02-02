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
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	jsoniter "github.com/json-iterator/go"
)

type SubmodelDescriptor struct {
	Administration types.IAdministrativeInformation `json:"administration,omitempty"`

	Endpoints []Endpoint `json:"endpoints"`

	IdShort string `json:"idShort,omitempty" validate:"regexp=^[a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9_]+$"`

	Id string `json:"id" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	SemanticId types.IReference `json:"semanticId,omitempty"`

	SupplementalSemanticId []types.IReference `json:"supplementalSemanticIds,omitempty"`

	Description []types.ILangStringTextType `json:"description,omitempty"`

	DisplayName []types.ILangStringNameType `json:"displayName,omitempty"`

	Extensions []types.Extension `json:"extensions,omitempty"`
}

// AssertSubmodelDescriptorRequired checks if the required fields are not zero-ed
func AssertSubmodelDescriptorRequired(obj SubmodelDescriptor) error {
	elements := map[string]any{
		"endpoints": obj.Endpoints,
		"id":        obj.Id,
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
	return nil
}

// AssertSubmodelDescriptorConstraints checks if the values respects the defined constraints
func AssertSubmodelDescriptorConstraints(obj SubmodelDescriptor) error {
	for _, el := range obj.Endpoints {
		if err := AssertEndpointConstraints(el); err != nil {
			return err
		}
	}
	return nil
}

func (obj SubmodelDescriptor) ToJsonable() (map[string]any, error) {
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

	// Supplemental Semantic IDs
	var supplementalSemanticIDs []map[string]any
	for _, ssm := range obj.SupplementalSemanticId {
		ssmMap, err := jsonization.ToJsonable(ssm)
		if err != nil {
			return nil, err
		}
		supplementalSemanticIDs = append(supplementalSemanticIDs, ssmMap)
	}

	// Extensions
	var extensions []map[string]any
	for _, ext := range obj.Extensions {
		extMap, err := jsonization.ToJsonable(&ext)
		if err != nil {
			return nil, err
		}
		extensions = append(extensions, extMap)
	}

	// Semantic ID
	var semanticID map[string]any
	if obj.SemanticId != nil {
		var err error
		semanticID, err = jsonization.ToJsonable(obj.SemanticId)
		if err != nil {
			return nil, err
		}
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
	if len(obj.Endpoints) > 0 {
		ret["endpoints"] = obj.Endpoints
	}
	if obj.IdShort != "" {
		ret["idShort"] = obj.IdShort
	}
	if obj.Id != "" {
		ret["id"] = obj.Id
	}
	if semanticID != nil {
		ret["semanticId"] = semanticID
	}
	if len(supplementalSemanticIDs) > 0 {
		ret["supplementalSemanticIds"] = supplementalSemanticIDs
	}
	return ret, nil
}

// UnmarshalJSON implements custom unmarshaling for SubmodelDescriptor
// It handles the FromJsonable Unmarshalling of the aas-go-sdk types
func (obj *SubmodelDescriptor) UnmarshalJSON(data []byte) error {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var jsonable map[string]any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return err
	}

	// Check for unknown fields
	allowedFields := map[string]bool{
		"administration":          true,
		"endpoints":               true,
		"idShort":                 true,
		"id":                      true,
		"semanticId":              true,
		"supplementalSemanticIds": true,
		"description":             true,
		"displayName":             true,
		"extensions":              true,
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
				return errors.New("SubmodelDescriptor: description is not a map")
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
				return errors.New("SubmodelDescriptor: displayName is not a map")
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
				return errors.New("SubmodelDescriptor: extension is not a map")
			}
			var extension types.IExtension
			extension, err := jsonization.ExtensionFromJsonable(extMap)
			if err != nil {
				return err
			}
			convExt, ok := extension.(*types.Extension)
			if !ok {
				return errors.New("SubmodelDescriptor: extension is not of type Extension")
			}
			if convExt == nil {
				return errors.New("SubmodelDescriptor: extension is nil")
			}
			obj.Extensions = append(obj.Extensions, *convExt)
		}
	}

	// Endpoints
	if eps, ok := jsonable["endpoints"].([]any); ok {
		for _, ep := range eps {
			epMap, ok := ep.(map[string]any)
			if !ok {
				return errors.New("SubmodelDescriptor: endpoint is not a map")
			}
			var endpoint Endpoint
			endpointBytes, err := json.Marshal(epMap)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(endpointBytes, &endpoint); err != nil {
				return err
			}
			obj.Endpoints = append(obj.Endpoints, endpoint)
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

	// Supplemental Semantic IDs
	if ssids, ok := jsonable["supplementalSemanticIds"].([]any); ok {
		for _, ssid := range ssids {
			ssidMap, ok := ssid.(map[string]any)
			if !ok {
				return errors.New("SubmodelDescriptor: supplementalSemanticId is not a map")
			}
			var reference types.IReference
			reference, err := jsonization.ReferenceFromJsonable(ssidMap)
			if err != nil {
				return err
			}
			obj.SupplementalSemanticId = append(obj.SupplementalSemanticId, reference)
		}
	}

	// Semantic ID
	if sid, ok := jsonable["semanticId"].(map[string]any); ok {
		var reference types.IReference
		reference, err := jsonization.ReferenceFromJsonable(sid)
		if err != nil {
			return err
		}
		obj.SemanticId = reference
	}

	// Handle other simple fields
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

	// Semantic ID
	validationErrors = []string{}
	if obj.SemanticId != nil {
		verification.Verify(obj.SemanticId, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})
		if len(validationErrors) > 0 {
			return errors.New("SubmodelDescriptor: SemanticId verification failed: " + validationErrors[0])
		}
	}

	// Supplemental Semantic IDs
	validationErrors = []string{}
	for _, el := range obj.SupplementalSemanticId {
		verification.Verify(el, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})
	}
	if len(validationErrors) > 0 {
		return errors.New("SubmodelDescriptor: SupplementalSemanticIds verification failed: " + validationErrors[0])
	}

	// Administration
	validationErrors = []string{}
	if obj.Administration != nil {
		verification.Verify(obj.Administration, func(verErr *verification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false // Continue collecting all errors
		})
		if len(validationErrors) > 0 {
			return errors.New("SubmodelDescriptor: Administration verification failed: " + validationErrors[0])
		}
	}

	return nil
}
