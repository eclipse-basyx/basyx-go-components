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

// package openapi
package api

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func newAPIErrorResponse(err error, status int, operation string, info string) gen.ImplResponse {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	return common.NewErrorResponse(err, status, componentName, operation, info)
}

func decodeBase64RawStd(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	decodedBytes, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}

	return string(decodedBytes), nil
}

func mergeJSONObjects(base map[string]any, patch map[string]any) map[string]any {
	merged := make(map[string]any, len(base))
	for key, value := range base {
		merged[key] = value
	}

	for key, patchValue := range patch {
		if patchValue == nil {
			delete(merged, key)
			continue
		}

		baseValue, baseExists := merged[key]
		baseMap, baseIsMap := baseValue.(map[string]any)
		patchMap, patchIsMap := patchValue.(map[string]any)
		if baseExists && baseIsMap && patchIsMap {
			merged[key] = mergeJSONObjects(baseMap, patchMap)
			continue
		}

		merged[key] = patchValue
	}

	return merged
}

func normalizeSubmodelLevel(level string) (string, error) {
	if level == "" {
		return "deep", nil
	}

	switch level {
	case "deep", "core":
		return level, nil
	default:
		return "", errors.New("invalid level, allowed values are 'deep' or 'core'")
	}
}

func normalizeSubmodelExtent(extent string) (string, error) {
	if extent == "" {
		return "withoutBlobValue", nil
	}

	switch extent {
	case "withoutBlobValue", "withBlobValue":
		return extent, nil
	default:
		return "", errors.New("invalid extent, allowed values are 'withoutBlobValue' or 'withBlobValue'")
	}
}

func applyCoreLevelToSubmodelJSON(submodelJSON map[string]any) {
	rawElements, exists := submodelJSON["submodelElements"]
	if !exists {
		return
	}

	elements, ok := rawElements.([]any)
	if !ok {
		return
	}

	for _, rawElement := range elements {
		element, ok := rawElement.(map[string]any)
		if !ok {
			continue
		}
		removeNestedContentFromSubmodelElementJSON(element)
	}
}

func removeNestedContentFromSubmodelElementJSON(element map[string]any) {
	rawModelType, exists := element["modelType"]
	if !exists {
		return
	}

	modelType, ok := rawModelType.(string)
	if !ok {
		return
	}

	switch modelType {
	case "SubmodelElementCollection", "SubmodelElementList":
		delete(element, "value")
	case "Entity":
		delete(element, "statements")
	case "AnnotatedRelationshipElement":
		delete(element, "annotations")
	case "Operation":
		delete(element, "inputVariables")
		delete(element, "outputVariables")
		delete(element, "inoutputVariables")
	}
}

func stripBlobValuesFromJSONTree(node any) {
	switch typedNode := node.(type) {
	case map[string]any:
		if modelType, ok := typedNode["modelType"].(string); ok && modelType == "Blob" {
			delete(typedNode, "value")
		}

		for _, value := range typedNode {
			stripBlobValuesFromJSONTree(value)
		}
	case []any:
		for _, item := range typedNode {
			stripBlobValuesFromJSONTree(item)
		}
	}
}

func applyLevelAndExtentToSubmodelJSON(submodelJSON map[string]any, level string, extent string) {
	if level == "core" {
		applyCoreLevelToSubmodelJSON(submodelJSON)
	}

	if extent == "withoutBlobValue" {
		stripBlobValuesFromJSONTree(submodelJSON)
	}
}
