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
	listType := types.AASSubmodelElementsProperty
	if len(values) > 0 {
		listType = listElementType(values[0])
	}
	list := types.NewSubmodelElementList(listType)
	list.SetIDShort(&idShort)

	elements := make([]types.ISubmodelElement, 0, len(values))
	for index, value := range values {
		element, err := inferElement("", value)
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

func listElementType(value any) types.AASSubmodelElements {
	if object, ok := value.(map[string]any); ok {
		if isFileObject(object) {
			return types.AASSubmodelElementsFile
		}
		return types.AASSubmodelElementsSubmodelElementCollection
	}
	return types.AASSubmodelElementsProperty
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

func numberProperty(idShort string, value json.Number) types.ISubmodelElement {
	if _, err := value.Int64(); err == nil {
		return scalarProperty(idShort, value.String(), types.DataTypeDefXSDLong)
	}
	return scalarProperty(idShort, value.String(), types.DataTypeDefXSDDouble)
}

func stringList(idShort string, values []string) types.ISubmodelElement {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	element, _ := listElement(idShort, items)
	return element
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
	return file
}

func isFileObject(values map[string]any) bool {
	_, hasURL := values["url"].(string)
	_, hasContentType := values["contentType"].(string)
	return hasURL && hasContentType
}
