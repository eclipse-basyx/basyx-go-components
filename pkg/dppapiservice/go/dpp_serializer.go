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
	content, err := dppCollectionFromSubmodel(submodel)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-FULL convert submodel to DPP expanded representation: %w", err)
	}
	return content, nil
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

func dppCollectionFromSubmodel(submodel types.ISubmodel) (map[string]any, error) {
	value, err := dppElementsFromAAS(submodel.SubmodelElements())
	if err != nil {
		return nil, err
	}
	result := dppElementBase(idShortOrID(submodel), "DataElementCollection", submodel.SemanticID())
	result["value"] = value
	return result, nil
}

func dppElementFromAAS(element types.ISubmodelElement) (map[string]any, error) {
	switch typed := element.(type) {
	case *types.Property:
		return singleValuedDataElement(typed), nil
	case *types.SubmodelElementList:
		return multiValuedDataElement(typed)
	case *types.MultiLanguageProperty:
		return multiLanguageDataElement(typed), nil
	case *types.SubmodelElementCollection:
		return dataElementCollection(typed)
	case *types.File:
		return relatedResource(typed), nil
	default:
		return nil, fmt.Errorf("DPP-ELEM-FULL-UNSUPPORTED unsupported AAS element type %v", element.ModelType())
	}
}

func singleValuedDataElement(property *types.Property) map[string]any {
	result := dppElementBase(idShortValue(property), "SingleValuedDataElement", property.SemanticID())
	result["valueDataType"] = dppValueType(property.ValueType())
	result["value"] = dereferenceString(property.Value())
	return result
}

func multiValuedDataElement(list *types.SubmodelElementList) (map[string]any, error) {
	result := dppElementBase(idShortValue(list), "MultiValuedDataElement", list.SemanticID())
	result["valueDataType"] = dppListValueType(list)
	values := make([]any, 0, len(list.Value()))
	for _, child := range list.Value() {
		value, err := dppListValue(child)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	result["value"] = values
	return result, nil
}

func multiLanguageDataElement(property *types.MultiLanguageProperty) map[string]any {
	result := dppElementBase(idShortValue(property), "MultiLanguageDataElement", property.SemanticID())
	values := make([]map[string]any, 0, len(property.Value()))
	for _, langString := range property.Value() {
		values = append(values, map[string]any{
			"language": langString.Language(),
			"value":    langString.Text(),
		})
	}
	result["value"] = values
	return result
}

func dataElementCollection(collection *types.SubmodelElementCollection) (map[string]any, error) {
	value, err := dppElementsFromAAS(collection.Value())
	if err != nil {
		return nil, err
	}
	result := dppElementBase(idShortValue(collection), "DataElementCollection", collection.SemanticID())
	result["value"] = value
	return result, nil
}

func relatedResource(file *types.File) map[string]any {
	result := dppElementBase(idShortValue(file), "RelatedResource", file.SemanticID())
	result["contentType"] = dereferenceString(file.ContentType())
	result["url"] = dereferenceString(file.Value())
	return result
}

func dppElementsFromAAS(elements []types.ISubmodelElement) ([]map[string]any, error) {
	value := make([]map[string]any, 0, len(elements))
	for _, element := range elements {
		mapped, err := dppElementFromAAS(element)
		if err != nil {
			return nil, err
		}
		value = append(value, mapped)
	}
	return value, nil
}

func dppElementBase(elementID string, objectType string, semanticID types.IReference) map[string]any {
	result := map[string]any{
		"elementId":  elementID,
		"objectType": objectType,
	}
	if dictionaryReference := referenceToString(semanticID); dictionaryReference != "" {
		result["dictionaryReference"] = dictionaryReference
	}
	return result
}

func idShortValue(value interface{ IDShort() *string }) string {
	if value.IDShort() == nil {
		return ""
	}
	return *value.IDShort()
}

func dppListValue(element types.ISubmodelElement) (any, error) {
	switch typed := element.(type) {
	case *types.Property:
		return dereferenceString(typed.Value()), nil
	case *types.File:
		return relatedResource(typed), nil
	case *types.MultiLanguageProperty:
		return multiLanguageDataElement(typed), nil
	case *types.SubmodelElementCollection:
		return dataElementCollection(typed)
	case *types.SubmodelElementList:
		return multiValuedDataElement(typed)
	default:
		return nil, fmt.Errorf("DPP-ELEM-FULL-UNSUPPORTEDLIST unsupported AAS list element type %v", element.ModelType())
	}
}

func dppListValueType(list *types.SubmodelElementList) string {
	if list.ValueTypeListElement() != nil {
		return dppValueType(*list.ValueTypeListElement())
	}
	for _, child := range list.Value() {
		if property, ok := child.(*types.Property); ok {
			return dppValueType(property.ValueType())
		}
	}
	return dppValueType(types.DataTypeDefXSDString)
}

func dppValueType(valueType types.DataTypeDefXSD) string {
	text := "string"
	switch valueType {
	case types.DataTypeDefXSDAnyURI:
		text = "anyURI"
	case types.DataTypeDefXSDBase64Binary:
		text = "base64Binary"
	case types.DataTypeDefXSDBoolean:
		text = "boolean"
	case types.DataTypeDefXSDByte:
		text = "byte"
	case types.DataTypeDefXSDDate:
		text = "date"
	case types.DataTypeDefXSDDateTime:
		text = "dateTime"
	case types.DataTypeDefXSDDecimal:
		text = "decimal"
	case types.DataTypeDefXSDDouble:
		text = "double"
	case types.DataTypeDefXSDDuration:
		text = "duration"
	case types.DataTypeDefXSDFloat:
		text = "float"
	case types.DataTypeDefXSDGDay:
		text = "gDay"
	case types.DataTypeDefXSDGMonth:
		text = "gMonth"
	case types.DataTypeDefXSDGMonthDay:
		text = "gMonthDay"
	case types.DataTypeDefXSDGYear:
		text = "gYear"
	case types.DataTypeDefXSDGYearMonth:
		text = "gYearMonth"
	case types.DataTypeDefXSDHexBinary:
		text = "hexBinary"
	case types.DataTypeDefXSDInt:
		text = "int"
	case types.DataTypeDefXSDInteger:
		text = "integer"
	case types.DataTypeDefXSDLong:
		text = "long"
	case types.DataTypeDefXSDNegativeInteger:
		text = "negativeInteger"
	case types.DataTypeDefXSDNonNegativeInteger:
		text = "nonNegativeInteger"
	case types.DataTypeDefXSDNonPositiveInteger:
		text = "nonPositiveInteger"
	case types.DataTypeDefXSDPositiveInteger:
		text = "positiveInteger"
	case types.DataTypeDefXSDShort:
		text = "short"
	case types.DataTypeDefXSDTime:
		text = "time"
	case types.DataTypeDefXSDUnsignedByte:
		text = "unsignedByte"
	case types.DataTypeDefXSDUnsignedInt:
		text = "unsignedInt"
	case types.DataTypeDefXSDUnsignedLong:
		text = "unsignedLong"
	case types.DataTypeDefXSDUnsignedShort:
		text = "unsignedShort"
	}
	return "xsd:" + text
}

func referenceToString(ref types.IReference) string {
	value := referenceLastValue(ref)
	if value == "" {
		return ""
	}
	return value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
