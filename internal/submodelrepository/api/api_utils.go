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
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type modelReferencePathSegment struct {
	value   string
	isIndex bool
}

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

func parseModelReferencePathSegments(idShortPath string) ([]modelReferencePathSegment, error) {
	if idShortPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPARAMS Invalid reference parameters")
	}

	segments := make([]modelReferencePathSegment, 0, 4)
	current := strings.Builder{}

	flushCurrent := func() {
		if current.Len() == 0 {
			return
		}
		segments = append(segments, modelReferencePathSegment{value: current.String()})
		current.Reset()
	}

	for i := 0; i < len(idShortPath); i++ {
		switch idShortPath[i] {
		case '.':
			flushCurrent()
		case '[':
			flushCurrent()
			endIndex := strings.IndexByte(idShortPath[i+1:], ']')
			if endIndex < 0 {
				return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Invalid idShort path syntax")
			}

			start := i + 1
			end := start + endIndex
			indexValue := idShortPath[start:end]
			if indexValue == "" {
				return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Empty list index in idShort path")
			}

			segments = append(segments, modelReferencePathSegment{value: indexValue, isIndex: true})
			i = end
		default:
			err := current.WriteByte(idShortPath[i])
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Invalid idShort path syntax")
			}
		}
	}

	flushCurrent()

	if len(segments) == 0 {
		return nil, common.NewErrBadRequest("SMREPO-BUILDREF-INVALIDPATH Invalid idShort path syntax")
	}

	return segments, nil
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

		if segment.isIndex {
			keyTypes = append(keyTypes, "SubmodelElementList")
			keyValues = append(keyValues, segment.value)

			currentPath += "[" + segment.value + "]"
			continue
		}

		if currentPath == "" {
			currentPath = segment.value
		} else {
			currentPath += "." + segment.value
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
		keyValues = append(keyValues, segment.value)
	}

	return keyTypes, keyValues, nil
}

func isLevelValid(level string) bool {
	return level == "core" || level == "" || level == "deep"
}
