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

package common

import (
	"fmt"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/stringification"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func parseStringField(raw any, field string) (string, error) {
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value, nil
}

func parseMapField(raw any, field string) (map[string]any, error) {
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return value, nil
}

func parseArrayField(raw any, field string) ([]any, error) {
	value, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", field)
	}
	return value, nil
}

func parseLangStringTextTypes(raw any, field string) ([]types.ILangStringTextType, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.ILangStringTextType, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.LangStringTextTypeFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseLangStringNameTypes(raw any, field string) ([]types.ILangStringNameType, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.ILangStringNameType, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.LangStringNameTypeFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseExtensions(raw any, field string) ([]types.IExtension, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.IExtension, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.ExtensionFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseAdministration(raw any, field string) (types.IAdministrativeInformation, error) {
	item, err := parseMapField(raw, field)
	if err != nil {
		return nil, err
	}

	return jsonization.AdministrativeInformationFromJsonable(item)
}

func parseEmbeddedDataSpecifications(raw any, field string) ([]types.IEmbeddedDataSpecification, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.IEmbeddedDataSpecification, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.EmbeddedDataSpecificationFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseQualifiers(raw any, field string) ([]types.IQualifier, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.IQualifier, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.QualifierFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseReference(raw any, field string) (types.IReference, error) {
	item, err := parseMapField(raw, field)
	if err != nil {
		return nil, err
	}

	return jsonization.ReferenceFromJsonable(item)
}

func parseReferences(raw any, field string) ([]types.IReference, error) {
	items, err := parseArrayField(raw, field)
	if err != nil {
		return nil, err
	}

	values := make([]types.IReference, 0, len(items))
	for _, item := range items {
		parsed, parseErr := jsonization.ReferenceFromJsonable(item)
		if parseErr != nil {
			return nil, parseErr
		}
		values = append(values, parsed)
	}

	return values, nil
}

func parseModelType(raw any, field string) (types.ModelType, error) {
	literal, err := parseStringField(raw, field)
	if err != nil {
		return 0, err
	}

	value, ok := stringification.ModelTypeFromString(literal)
	if !ok {
		return 0, fmt.Errorf("%s has unsupported value '%s'", field, literal)
	}

	return value, nil
}

func parseModellingKind(raw any, field string) (types.ModellingKind, error) {
	literal, err := parseStringField(raw, field)
	if err != nil {
		return 0, err
	}

	value, ok := stringification.ModellingKindFromString(literal)
	if !ok {
		return 0, fmt.Errorf("%s has unsupported value '%s'", field, literal)
	}

	return value, nil
}

func parseMetadataPayload(raw any) (map[string]any, error) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata payload must be an object")
	}
	return payload, nil
}

// ParseSubmodelMetadataPayload parses metadata PATCH payloads into SubmodelMetadata.
func ParseSubmodelMetadataPayload(raw any) (model.SubmodelMetadata, error) {
	payload, err := parseMetadataPayload(raw)
	if err != nil {
		return model.SubmodelMetadata{}, fmt.Errorf("SMREPO-PARSESMMETA-INVALIDPAYLOAD %w", err)
	}

	metadata := model.SubmodelMetadata{}
	for field, value := range payload {
		switch field {
		case "extensions":
			metadata.Extensions, err = parseExtensions(value, field)
		case "category":
			metadata.Category, err = parseStringField(value, field)
		case "idShort":
			metadata.IdShort, err = parseStringField(value, field)
		case "displayName":
			metadata.DisplayName, err = parseLangStringNameTypes(value, field)
		case "description":
			metadata.Description, err = parseLangStringTextTypes(value, field)
		case "modelType":
			metadata.ModelType, err = parseModelType(value, field)
		case "administration":
			metadata.Administration, err = parseAdministration(value, field)
		case "id":
			metadata.ID, err = parseStringField(value, field)
		case "embeddedDataSpecifications":
			metadata.EmbeddedDataSpecifications, err = parseEmbeddedDataSpecifications(value, field)
		case "qualifiers":
			metadata.Qualifiers, err = parseQualifiers(value, field)
		case "semanticId":
			metadata.SemanticID, err = parseReference(value, field)
		case "supplementalSemanticIds":
			metadata.SupplementalSemanticIds, err = parseReferences(value, field)
		case "kind":
			metadata.Kind, err = parseModellingKind(value, field)
		default:
			return model.SubmodelMetadata{}, fmt.Errorf("SMREPO-PARSESMMETA-UNKNOWNFIELD unknown metadata field '%s'", field)
		}

		if err != nil {
			return model.SubmodelMetadata{}, fmt.Errorf("SMREPO-PARSESMMETA-INVALIDFIELD invalid metadata field '%s': %w", field, err)
		}
	}

	if metadata.ModelType == 0 {
		return model.SubmodelMetadata{}, fmt.Errorf("SMREPO-PARSESMMETA-MISSINGMODELTYPE metadata payload requires modelType")
	}

	return metadata, nil
}

// ParseSubmodelElementMetadataPayload parses metadata PATCH payloads into SubmodelElementMetadata.
func ParseSubmodelElementMetadataPayload(raw any) (model.SubmodelElementMetadata, error) {
	payload, err := parseMetadataPayload(raw)
	if err != nil {
		return model.SubmodelElementMetadata{}, fmt.Errorf("SMREPO-PARSESMEMETA-INVALIDPAYLOAD %w", err)
	}

	metadata := model.SubmodelElementMetadata{}
	for field, value := range payload {
		switch field {
		case "extensions":
			metadata.Extensions, err = parseExtensions(value, field)
		case "category":
			metadata.Category, err = parseStringField(value, field)
		case "idShort":
			metadata.IdShort, err = parseStringField(value, field)
		case "displayName":
			metadata.DisplayName, err = parseLangStringNameTypes(value, field)
		case "description":
			metadata.Description, err = parseLangStringTextTypes(value, field)
		case "modelType":
			metadata.ModelType, err = parseModelType(value, field)
		case "embeddedDataSpecifications":
			metadata.EmbeddedDataSpecifications, err = parseEmbeddedDataSpecifications(value, field)
		case "semanticId":
			metadata.SemanticID, err = parseReference(value, field)
		case "supplementalSemanticIds":
			metadata.SupplementalSemanticIds, err = parseReferences(value, field)
		case "qualifiers":
			metadata.Qualifiers, err = parseQualifiers(value, field)
		case "kind":
			metadata.Kind, err = parseModellingKind(value, field)
		default:
			return model.SubmodelElementMetadata{}, fmt.Errorf("SMREPO-PARSESMEMETA-UNKNOWNFIELD unknown metadata field '%s'", field)
		}

		if err != nil {
			return model.SubmodelElementMetadata{}, fmt.Errorf("SMREPO-PARSESMEMETA-INVALIDFIELD invalid metadata field '%s': %w", field, err)
		}
	}

	if metadata.ModelType == 0 {
		return model.SubmodelElementMetadata{}, fmt.Errorf("SMREPO-PARSESMEMETA-MISSINGMODELTYPE metadata payload requires modelType")
	}

	return metadata, nil
}
