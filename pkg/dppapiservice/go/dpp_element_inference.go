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
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/types"
)

const (
	dppResourceTitleExtensionName = "dppResourceTitle"
	dppLanguageExtensionName      = "dppLanguage"
)

type compactArrayKind int

const (
	compactArrayScalar compactArrayKind = iota
	compactArrayCollection
	compactArrayFile
)

type compactArrayClassification struct {
	kind      compactArrayKind
	valueType types.DataTypeDefXSD
}

func inferElement(idShort string, value any) (types.ISubmodelElement, error) {
	switch typed := value.(type) {
	case map[string]any:
		if isFileObject(typed) {
			return fileElement(idShort, typed), nil
		}
		return collectionElement(idShort, typed)
	case []any:
		return listElement(idShort, typed)
	case string:
		return scalarProperty(idShort, typed, types.DataTypeDefXSDString), nil
	case bool:
		return scalarProperty(idShort, strconv.FormatBool(typed), types.DataTypeDefXSDBoolean), nil
	case json.Number:
		return numberProperty(idShort, typed), nil
	case nil:
		return scalarProperty(idShort, "", types.DataTypeDefXSDString), nil
	default:
		return scalarProperty(idShort, fmt.Sprint(typed), types.DataTypeDefXSDString), nil
	}
}

func collectionElement(idShort string, values map[string]any) (types.ISubmodelElement, error) {
	collection := types.NewSubmodelElementCollection()
	collection.SetIDShort(&idShort)
	keys := sortedKeys(values)
	elements := make([]types.ISubmodelElement, 0, len(keys))
	for _, key := range keys {
		element, err := inferElement(key, values[key])
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
	collection.SetValue(elements)
	return collection, nil
}

func listElement(idShort string, values []any) (types.ISubmodelElement, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("DPP-ELEM-EMPTYARRAY array element %s must not be empty without an explicit data type", idShort)
	}
	if hasMultiLanguageValue(values) {
		return multiLanguageElement(idShort, values)
	}
	classification, err := classifyCompactArray(idShort, values)
	if err != nil {
		return nil, err
	}
	listType := listElementType(classification)
	list := types.NewSubmodelElementList(listType)
	list.SetIDShort(&idShort)
	if classification.kind == compactArrayScalar {
		valueType := classification.valueType
		list.SetValueTypeListElement(&valueType)
	}

	elements := make([]types.ISubmodelElement, 0, len(values))
	for index, value := range values {
		element, err := inferListElement(idShort, index, value, classification)
		if err != nil {
			return nil, err
		}
		if idShorter, ok := element.(interface{ SetIDShort(*string) }); ok {
			empty := ""
			if listType == types.AASSubmodelElementsSubmodelElementCollection {
				empty = fmt.Sprintf("%s%d", idShort, index)
			}
			idShorter.SetIDShort(&empty)
		}
		elements = append(elements, element)
	}
	list.SetValue(elements)
	return list, nil
}

func inferListElement(idShort string, index int, value any, classification compactArrayClassification) (types.ISubmodelElement, error) {
	if classification.kind == compactArrayScalar {
		return scalarListProperty(value, classification.valueType)
	}
	element, err := inferElement("", value)
	if err != nil {
		return nil, err
	}
	if classification.kind == compactArrayCollection {
		if idShorter, ok := element.(interface{ SetIDShort(*string) }); ok {
			itemIDShort := fmt.Sprintf("%s%d", idShort, index)
			idShorter.SetIDShort(&itemIDShort)
		}
	}
	return element, nil
}

func listElementType(classification compactArrayClassification) types.AASSubmodelElements {
	switch classification.kind {
	case compactArrayCollection:
		return types.AASSubmodelElementsSubmodelElementCollection
	case compactArrayFile:
		return types.AASSubmodelElementsFile
	default:
		return types.AASSubmodelElementsProperty
	}
}

func stringProperty(idShort string, value string) types.ISubmodelElement {
	return scalarProperty(idShort, value, types.DataTypeDefXSDString)
}

func scalarProperty(idShort string, value string, valueType types.DataTypeDefXSD) types.ISubmodelElement {
	property := types.NewProperty(valueType)
	property.SetIDShort(&idShort)
	property.SetValue(&value)
	return property
}

func scalarListProperty(value any, valueType types.DataTypeDefXSD) (types.ISubmodelElement, error) {
	switch typed := value.(type) {
	case string:
		return scalarProperty("", typed, valueType), nil
	case bool:
		return scalarProperty("", strconv.FormatBool(typed), valueType), nil
	case json.Number:
		return scalarProperty("", typed.String(), valueType), nil
	default:
		return nil, fmt.Errorf("DPP-ELEM-ARRAYTYPE unsupported scalar list value %v", value)
	}
}

func numberProperty(idShort string, value json.Number) types.ISubmodelElement {
	return scalarProperty(idShort, value.String(), numberValueType(value))
}

func stringList(idShort string, values []string) types.ISubmodelElement {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	element, _ := listElement(idShort, items)
	return element
}

func multiLanguageElement(idShort string, values []any) (types.ISubmodelElement, error) {
	property := types.NewMultiLanguageProperty()
	property.SetIDShort(&idShort)
	langStrings := make([]types.ILangStringTextType, 0, len(values))
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok || !isMultiLanguageValueObject(object) {
			return nil, fmt.Errorf("DPP-ELEM-MIXEDARRAY multilingual array %s must contain only language/value objects", idShort)
		}
		langStrings = append(langStrings, types.NewLangStringTextType(object["language"].(string), object["value"].(string)))
	}
	property.SetValue(langStrings)
	return property, nil
}

func fileElement(idShort string, values map[string]any) types.ISubmodelElement {
	file := types.NewFile()
	file.SetIDShort(&idShort)
	if url, ok := values["url"].(string); ok {
		file.SetValue(&url)
	}
	if contentType, ok := values["contentType"].(string); ok {
		file.SetContentType(&contentType)
	}
	file.SetExtensions(dppFileExtensions(values))
	return file
}

func isFileObject(values map[string]any) bool {
	_, hasURL := values["url"].(string)
	_, hasContentType := values["contentType"].(string)
	return hasURL && hasContentType
}

func dppFileExtensions(values map[string]any) []types.IExtension {
	extensions := make([]types.IExtension, 0, 2)
	if resourceTitle, ok := nonEmptyString(values["resourceTitle"]); ok {
		extensions = append(extensions, stringExtension(dppResourceTitleExtensionName, resourceTitle))
	}
	if language, ok := nonEmptyString(values["language"]); ok {
		extensions = append(extensions, stringExtension(dppLanguageExtensionName, language))
	}
	return extensions
}

func stringExtension(name string, value string) types.IExtension {
	extension := types.NewExtension(name)
	valueType := types.DataTypeDefXSDString
	extension.SetValueType(&valueType)
	extension.SetValue(&value)
	return extension
}

func nonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok && text != ""
}

func hasMultiLanguageValue(values []any) bool {
	for _, value := range values {
		object, ok := value.(map[string]any)
		if ok && isMultiLanguageValueObject(object) {
			return true
		}
	}
	return false
}

func isMultiLanguageValueObject(values map[string]any) bool {
	if len(values) != 2 {
		return false
	}
	_, hasLanguage := values["language"].(string)
	_, hasValue := values["value"].(string)
	return hasLanguage && hasValue
}

func classifyCompactArray(idShort string, values []any) (compactArrayClassification, error) {
	classification, err := compactArrayItemClassification(values[0])
	if err != nil {
		return compactArrayClassification{}, fmt.Errorf("DPP-ELEM-ARRAYTYPE array %s: %w", idShort, err)
	}
	for _, value := range values[1:] {
		next, err := compactArrayItemClassification(value)
		if err != nil {
			return compactArrayClassification{}, fmt.Errorf("DPP-ELEM-ARRAYTYPE array %s: %w", idShort, err)
		}
		if classification.kind != next.kind {
			return compactArrayClassification{}, fmt.Errorf("DPP-ELEM-MIXEDARRAY array %s must contain only one JSON data shape", idShort)
		}
		if classification.kind == compactArrayScalar {
			valueType, ok := compatibleScalarArrayValueType(classification.valueType, next.valueType)
			if !ok {
				return compactArrayClassification{}, fmt.Errorf("DPP-ELEM-MIXEDARRAY array %s must contain scalar values of one JSON data type", idShort)
			}
			classification.valueType = valueType
		}
	}
	return classification, nil
}

func compactArrayItemClassification(value any) (compactArrayClassification, error) {
	switch typed := value.(type) {
	case map[string]any:
		if isFileObject(typed) {
			return compactArrayClassification{kind: compactArrayFile}, nil
		}
		return compactArrayClassification{kind: compactArrayCollection}, nil
	case string:
		return compactArrayClassification{kind: compactArrayScalar, valueType: types.DataTypeDefXSDString}, nil
	case bool:
		return compactArrayClassification{kind: compactArrayScalar, valueType: types.DataTypeDefXSDBoolean}, nil
	case json.Number:
		return compactArrayClassification{kind: compactArrayScalar, valueType: numberValueType(typed)}, nil
	default:
		return compactArrayClassification{}, fmt.Errorf("unsupported item %v", value)
	}
}

func compatibleScalarArrayValueType(current types.DataTypeDefXSD, next types.DataTypeDefXSD) (types.DataTypeDefXSD, bool) {
	if current == next {
		return current, true
	}
	if isNumericValueType(current) && isNumericValueType(next) {
		return types.DataTypeDefXSDDouble, true
	}
	return current, false
}

func isNumericValueType(valueType types.DataTypeDefXSD) bool {
	return valueType == types.DataTypeDefXSDLong || valueType == types.DataTypeDefXSDDouble
}

func numberValueType(value json.Number) types.DataTypeDefXSD {
	if _, err := value.Int64(); err == nil {
		return types.DataTypeDefXSDLong
	}
	return types.DataTypeDefXSDDouble
}
