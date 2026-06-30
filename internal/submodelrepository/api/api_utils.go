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

package api

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelpath "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/path"
)

func buildModelReference(submodelID string, keyTypes []string, keyValues []string) (types.IReference, error) {
	if submodelID == "" || len(keyTypes) == 0 || len(keyValues) == 0 || len(keyTypes) != len(keyValues) {
		return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPARAMS Invalid reference parameters")
	}

	keys := make([]any, 0, len(keyTypes)+1)
	keys = append(keys, map[string]any{
		"type":  "Submodel",
		"value": submodelID,
	})

	for i := range keyTypes {
		if keyTypes[i] == "" || keyValues[i] == "" {
			return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPARAMS Invalid reference key parameters")
		}

		keys = append(keys, map[string]any{
			"type":  keyTypes[i],
			"value": keyValues[i],
		})
	}

	jsonableReference := map[string]any{
		"type": "ModelReference",
		"keys": keys,
	}

	return jsonization.ReferenceFromJsonable(jsonableReference)
}

func parseModelReferencePathSegments(idShortPath string) ([]submodelpath.Segment, error) {
	segments, err := submodelpath.ParseIDShortPathSegments(idShortPath)
	if err == nil {
		return segments, nil
	}

	if errors.Is(err, submodelpath.ErrEmptyPath) {
		return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPARAMS Invalid reference parameters")
	}
	if errors.Is(err, submodelpath.ErrEmptyListIndex) {
		return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Empty list index in idShort path")
	}
	return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Invalid idShort path syntax")
}

func resolveModelReferencePathKeys(
	idShortPath string,
	finalModelType string,
	resolvePathModelType func(path string) (string, error),
) ([]string, []string, error) {
	if finalModelType == "" {
		return nil, nil, common.NewInternalServerError("SMREPO-BUILDREF-EMPTYFINALTYPE Empty modelType for target submodel element")
	}

	segments, parseErr := parseModelReferencePathSegments(idShortPath)
	if parseErr != nil {
		return nil, nil, parseErr
	}

	keyTypes := make([]string, 0, len(segments))
	keyValues := make([]string, 0, len(segments))
	currentPath := ""

	for i, segment := range segments {
		isLast := i == len(segments)-1

		if segment.IsIndex {
			keyTypes = append(keyTypes, "SubmodelElementList")
			keyValues = append(keyValues, segment.Value)

			currentPath += "[" + segment.Value + "]"
			continue
		}

		if currentPath == "" {
			currentPath = segment.Value
		} else {
			currentPath += "." + segment.Value
		}

		keyType := finalModelType
		if !isLast {
			resolvedModelType, resolveErr := resolvePathModelType(currentPath)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			if resolvedModelType == "" {
				return nil, nil, common.NewInternalServerError("SMREPO-BUILDREF-EMPTYPARENTTYPE Empty modelType for parent submodel element")
			}
			keyType = resolvedModelType
		}

		keyTypes = append(keyTypes, keyType)
		keyValues = append(keyValues, segment.Value)
	}

	return keyTypes, keyValues, nil
}

func isLevelValid(level string) bool {
	return level == "core" || level == "" || level == "deep"
}

const (
	extentWithBlobValue    = "withBlobValue"
	extentWithoutBlobValue = "withoutBlobValue"
)

func validateLevel(level string) error {
	if isLevelValid(level) {
		return nil
	}
	return common.NewErrBadRequest("SMREPO-VALIDLEVEL-BADVALUE invalid level parameter")
}

func normalizeExtent(extent string) (string, error) {
	if extent == "" || extent == extentWithoutBlobValue {
		return extentWithoutBlobValue, nil
	}
	if extent == extentWithBlobValue {
		return extentWithBlobValue, nil
	}
	return "", common.NewErrBadRequest("SMREPO-NORMEXT-BADVALUE invalid extent parameter")
}

func stripBlobValuesFromSubmodel(submodel types.ISubmodel) {
	if submodel == nil {
		return
	}
	stripBlobValuesFromElements(submodel.SubmodelElements())
}

func stripBlobValuesFromElements(elements []types.ISubmodelElement) {
	for _, element := range elements {
		stripBlobValuesFromElement(element)
	}
}

func stripBlobValuesFromElement(element types.ISubmodelElement) {
	if element == nil {
		return
	}

	if blob, ok := element.(types.IBlob); ok {
		blob.SetValue(nil)
		return
	}

	switch typedElement := element.(type) {
	case types.ISubmodelElementList:
		stripBlobValuesFromElements(typedElement.Value())
	case types.ISubmodelElementCollection:
		stripBlobValuesFromElements(typedElement.Value())
	case types.IEntity:
		stripBlobValuesFromElements(typedElement.Statements())
	case types.IAnnotatedRelationshipElement:
		stripBlobValuesFromAnnotations(typedElement.Annotations())
	case types.IOperation:
		stripBlobValuesFromOperationVariables(typedElement.InputVariables())
		stripBlobValuesFromOperationVariables(typedElement.OutputVariables())
		stripBlobValuesFromOperationVariables(typedElement.InoutputVariables())
	}
}

func stripBlobValuesFromAnnotations(annotations []types.IDataElement) {
	for _, annotation := range annotations {
		stripBlobValuesFromElement(annotation)
	}
}

func stripBlobValuesFromOperationVariables(variables []types.IOperationVariable) {
	for _, variable := range variables {
		if variable == nil {
			continue
		}
		stripBlobValuesFromElement(variable.Value())
	}
}

func pruneSubmodelToCore(submodel types.ISubmodel) {
	if submodel == nil {
		return
	}
	pruneElementsToCore(submodel.SubmodelElements())
}

func pruneElementsToCore(elements []types.ISubmodelElement) {
	for _, element := range elements {
		pruneElementToCore(element)
	}
}

func pruneElementToCore(element types.ISubmodelElement) {
	if element == nil {
		return
	}

	switch typedElement := element.(type) {
	case types.ISubmodelElementList:
		typedElement.SetValue(nil)
	case types.ISubmodelElementCollection:
		typedElement.SetValue(nil)
	case types.IEntity:
		typedElement.SetStatements(nil)
	case types.IAnnotatedRelationshipElement:
		typedElement.SetAnnotations(nil)
	case types.IOperation:
		typedElement.SetInputVariables(nil)
		typedElement.SetOutputVariables(nil)
		typedElement.SetInoutputVariables(nil)
	}
}

func extractSubmodelIdentifierFromReference(reference types.IReference) (string, error) {
	if reference == nil {
		return "", common.NewInternalServerError("SMREPO-EXTRACTSMID-NILREFERENCE Submodel reference is nil")
	}

	keys := reference.Keys()
	if len(keys) == 0 {
		return "", common.NewInternalServerError("SMREPO-EXTRACTSMID-EMPTYKEYS Submodel reference contains no keys")
	}

	firstKey := keys[0]
	if firstKey == nil {
		return "", common.NewInternalServerError("SMREPO-EXTRACTSMID-NILKEY First reference key is nil")
	}

	if firstKey.Type() != types.KeyTypesSubmodel {
		return "", common.NewInternalServerError("SMREPO-EXTRACTSMID-BADKEYTYPE First reference key is not of type Submodel")
	}

	submodelIdentifier := firstKey.Value()
	if submodelIdentifier == "" {
		return "", common.NewInternalServerError("SMREPO-EXTRACTSMID-EMPTYVALUE Submodel reference key value is empty")
	}

	return submodelIdentifier, nil
}

type allSubmodelsPathCursorState struct {
	SubmodelCursor string `json:"submodelCursor,omitempty"`
	PathCursor     string `json:"pathCursor,omitempty"`
}

func decodeAllSubmodelsPathCursorState(cursor string) allSubmodelsPathCursorState {
	if strings.TrimSpace(cursor) == "" {
		return allSubmodelsPathCursorState{}
	}

	var state allSubmodelsPathCursorState
	if unmarshalErr := json.Unmarshal([]byte(cursor), &state); unmarshalErr == nil {
		if state.SubmodelCursor != "" || state.PathCursor != "" {
			return state
		}
	}

	return allSubmodelsPathCursorState{SubmodelCursor: cursor}
}

func encodeAllSubmodelsPathCursorState(state allSubmodelsPathCursorState) (string, error) {
	if state.SubmodelCursor == "" && state.PathCursor == "" {
		return "", nil
	}
	if state.PathCursor == "" {
		return state.SubmodelCursor, nil
	}

	payload, marshalErr := json.Marshal(state)
	if marshalErr != nil {
		return "", common.NewInternalServerError("SMREPO-ENCPATHCURSOR-MARSHAL " + marshalErr.Error())
	}
	return string(payload), nil
}
