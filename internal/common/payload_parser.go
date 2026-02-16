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
// Author: Jannik Fried ( Fraunhofer IESE )

package common

import (
	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	jsoniter "github.com/json-iterator/go"
)

var (
	jsonP = jsoniter.ConfigCompatibleWithStandardLibrary
)

func ParseReferenceJSON(rawPayload []byte) (types.IReference, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESEMJSON " + err.Error())
	}

	toReference := func(input map[string]any) (types.IReference, error) {
		if len(input) == 0 {
			return nil, nil
		}
		ref, err := jsonization.ReferenceFromJsonable(input)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESEMREF " + err.Error())
		}
		return ref, nil
	}

	switch v := payload.(type) {
	case map[string]any:
		return toReference(v)
	case []any:
		if len(v) == 0 {
			return nil, nil
		}
		first, ok := v[0].(map[string]any)
		if !ok {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESEMJSON unexpected array element type")
		}
		return toReference(first)
	default:
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESEMJSON unexpected payload type")
	}
}

func ParseReferencesJSONArray(rawPayload []byte) ([]types.IReference, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESUPPLJSON " + err.Error())
	}

	refs := make([]types.IReference, 0, len(payload))
	for _, item := range payload {
		ref, err := jsonization.ReferenceFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSESUPPLREF " + err.Error())
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

func ParseLangStringTextTypesJSON(rawPayload []byte) ([]types.ILangStringTextType, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEDESCJSON " + err.Error())
	}

	values := make([]types.ILangStringTextType, 0, len(payload))
	for _, item := range payload {
		value, err := jsonization.LangStringTextTypeFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEDESCVALUE " + err.Error())
		}
		values = append(values, value)
	}

	return values, nil
}

func ParseLangStringNameTypesJSON(rawPayload []byte) ([]types.ILangStringNameType, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEDISPNAMEJSON " + err.Error())
	}

	values := make([]types.ILangStringNameType, 0, len(payload))
	for _, item := range payload {
		value, err := jsonization.LangStringNameTypeFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEDISPNAMEVALUE " + err.Error())
		}
		values = append(values, value)
	}

	return values, nil
}

func ParseAdministrationJSON(rawPayload []byte) (types.IAdministrativeInformation, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEADMINJSON " + err.Error())
	}
	if len(payload) == 0 {
		return nil, nil
	}

	admin, err := jsonization.AdministrativeInformationFromJsonable(payload[0])
	if err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEADMINVALUE " + err.Error())
	}
	return admin, nil
}

func ParseEmbeddedDataSpecificationsJSON(rawPayload []byte) ([]types.IEmbeddedDataSpecification, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEEDSJSON " + err.Error())
	}

	values := make([]types.IEmbeddedDataSpecification, 0, len(payload))
	for _, item := range payload {
		value, err := jsonization.EmbeddedDataSpecificationFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEEDSVALUE " + err.Error())
		}
		values = append(values, value)
	}

	return values, nil
}

func ParseExtensionsJSON(rawPayload []byte) ([]types.IExtension, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEEXTJSON " + err.Error())
	}

	values := make([]types.IExtension, 0, len(payload))
	for _, item := range payload {
		value, err := jsonization.ExtensionFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEEXTVALUE " + err.Error())
		}
		values = append(values, value)
	}

	return values, nil
}

func ParseQualifiersJSON(rawPayload []byte) ([]types.IQualifier, error) {
	if len(rawPayload) == 0 {
		return nil, nil
	}

	var payload []map[string]any
	if err := jsonP.Unmarshal(rawPayload, &payload); err != nil {
		return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEQUALJSON " + err.Error())
	}

	values := make([]types.IQualifier, 0, len(payload))
	for _, item := range payload {
		value, err := jsonization.QualifierFromJsonable(item)
		if err != nil {
			return nil, NewInternalServerError("SMREPO-NEWSM-GETBYID-PARSEQUALVALUE " + err.Error())
		}
		values = append(values, value)
	}

	return values, nil
}
