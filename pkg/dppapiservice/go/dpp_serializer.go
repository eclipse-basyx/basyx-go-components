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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	basyxmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func composeHeader(metadata types.ISubmodel) (dppDocument, error) {
	valueOnly, err := basyxmodel.SubmodelToValueOnly(metadata)
	if err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAVALUE convert metadata value-only: %w", err)
	}
	raw, err := json.Marshal(valueOnly)
	if err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAMARSHAL marshal metadata value-only: %w", err)
	}
	var header dppDocument
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAUNMARSHAL unmarshal metadata value-only: %w", err)
	}
	normalizeValueOnly(header)
	return header, nil
}

func compressedContent(submodel types.ISubmodel) (any, error) {
	valueOnly, err := basyxmodel.SubmodelToValueOnly(submodel)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-COMPRESSED convert submodel value-only: %w", err)
	}
	raw, err := json.Marshal(valueOnly)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-MARSHAL marshal submodel value-only: %w", err)
	}
	var content any
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-UNMARSHAL unmarshal submodel value-only: %w", err)
	}
	normalizeValueOnly(content)
	return content, nil
}

func fullContent(submodel types.ISubmodel) (any, error) {
	jsonable, err := jsonization.ToJsonable(submodel)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-FULL convert submodel normal serialization: %w", err)
	}
	return aasNormalToDPPExpanded(jsonable), nil
}

func normalizeValueOnly(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if rawValue, ok := typed["value"]; ok && typed["contentType"] != nil {
			typed["url"] = rawValue
			delete(typed, "value")
		}
		for _, child := range typed {
			normalizeValueOnly(child)
		}
	case []any:
		for index, child := range typed {
			if langMap, ok := child.(map[string]any); ok && len(langMap) == 1 {
				for language, text := range langMap {
					typed[index] = map[string]any{"language": language, "value": text}
				}
			}
			normalizeValueOnly(typed[index])
		}
	}
}

func aasNormalToDPPExpanded(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			switch key {
			case "idShort":
				result["elementId"] = child
			case "modelType":
				result["objectType"] = child
			case "semanticId":
				result["dictionaryReference"] = referenceJSONToString(child)
			case "valueType":
				result["valueDataType"] = aasValueTypeToXSD(child)
			case "submodelElements":
				result["elements"] = aasNormalToDPPExpanded(child)
			case "contentType":
				result[key] = child
			case "value":
				result[key] = aasNormalToDPPExpanded(child)
			default:
				result[key] = aasNormalToDPPExpanded(child)
			}
		}
		return result
	case []any:
		items := make([]any, 0, len(typed))
		for _, child := range typed {
			items = append(items, aasNormalToDPPExpanded(child))
		}
		return items
	default:
		return value
	}
}

func referenceJSONToString(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	keys, ok := object["keys"].([]any)
	if !ok || len(keys) == 0 {
		return value
	}
	lastKey, ok := keys[len(keys)-1].(map[string]any)
	if !ok {
		return value
	}
	if keyValue, ok := lastKey["value"].(string); ok {
		return keyValue
	}
	return value
}

func aasValueTypeToXSD(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	text = strings.TrimPrefix(text, "xs:")
	text = strings.TrimPrefix(text, "xsd:")
	return "xsd:" + strings.ToLower(text[:1]) + text[1:]
}
